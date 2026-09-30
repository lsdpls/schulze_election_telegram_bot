package schulze

import (
	"context"
	"errors"
	"testing"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/config"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"
	mocks "github.com/lsdpls/schulze_election_telegram_bot/internal/schulze/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cands(ids ...int) []models.Candidate {
	out := make([]models.Candidate, len(ids))
	for i, id := range ids {
		out[i] = models.Candidate{CandidateID: id, Name: "Кандидат", Course: "1 бакалавриат", IsEligible: true}
	}
	return out
}

func ballots(rankings ...[]int) []models.Vote {
	out := make([]models.Vote, len(rankings))
	for i, r := range rankings {
		out[i] = models.Vote{ID: i + 1, DelegateID: 100000 + i, CandidateRankings: r}
	}
	return out
}

func setTotalPlaces(t *testing.T, n int) {
	t.Helper()
	old := config.TotalPlaces
	config.TotalPlaces = n
	t.Cleanup(func() { config.TotalPlaces = old })
}

// ID снятого или неизвестного кандидата в бюллетене не учитывается и не вызывает панику
func TestComputePairwisePreferencesIgnoresUnknownIDs(t *testing.T) {
	s := &Schulze{}
	got := s.computePairwisePreferences(ballots([]int{9, 1, 2}, []int{1, 9, 2}, []int{2, 1, 7}), cands(1, 2))
	assert.Equal(t, map[int]map[int]int{1: {2: 2}, 2: {1: 1}}, got)
}

// Общие места: из бюллетеней убираются победители курсов И кандидаты вне списка допущенных (снятые после голосования)
func TestExcludeCourseWinnersDropsIneligibleIDs(t *testing.T) {
	setTotalPlaces(t, 3)
	ch := mocks.NewMockchain(gomock.NewController(t))
	ch.EXPECT().GetAllResults(gomock.Any()).Return([]models.Result{{Course: "1 бакалавриат", WinnerCandidateID: []int{1}, Stage: "absolute"}}, nil)
	s := &Schulze{voteChain: ch}
	// 9 снят: его нет в allCandidates, но он остался в бюллетенях
	gotCands, gotVotes, places, err := s.excludeCourseWinners(context.Background(), cands(1, 2, 3), ballots([]int{9, 1, 2, 3}, []int{3, 9, 2, 1}))
	require.NoError(t, err)
	assert.Equal(t, 2, places)
	assert.Equal(t, cands(2, 3), gotCands)
	require.Len(t, gotVotes, 2)
	assert.Equal(t, []int{2, 3}, gotVotes[0].CandidateRankings)
	assert.Equal(t, []int{3, 2}, gotVotes[1].CandidateRankings)
}

// Кандидат снят после голосования: общие места считаются без паники, голоса за него не учитываются
func TestComputeGlobalTopWithBannedCandidateInBallots(t *testing.T) {
	setTotalPlaces(t, 2)
	ch := mocks.NewMockchain(gomock.NewController(t))
	ch.EXPECT().GetAllResults(gomock.Any()).Return([]models.Result{{Course: "1 бакалавриат", WinnerCandidateID: []int{1}, Stage: "absolute"}}, nil)
	var saved models.Result
	ch.EXPECT().AddResult(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r models.Result) error { saved = r; return nil })
	s := &Schulze{voteChain: ch, candidates: cands(1, 2, 3)}
	// 9 снят; без учёта 9 кандидат 3 выше 2 в двух бюллетенях из трёх
	s.votes = ballots([]int{9, 1, 3, 2}, []int{9, 3, 2, 1}, []int{2, 9, 3, 1})
	require.NoError(t, s.ComputeGlobalTop(context.Background()))
	assert.Equal(t, "Общие места", saved.Course)
	assert.Equal(t, []int{3}, saved.WinnerCandidateID)
	assert.NotContains(t, saved.Preferences, 9)
}

// Тай-брейк разрешил ничью за общее место — место получает его победитель, а не первый по порядку строк БД
// (раньше результат тай-брейка терялся из-за := и место доставалось кандидату 3)
func TestBuildStrictOrderUsesTieBreakWinner(t *testing.T) {
	s := &Schulze{}
	candidates := cands(1, 2, 3, 4, 5, 6)
	votes := ballots([]int{3, 5, 2, 1, 4, 6}, []int{5, 4, 3, 6, 1, 2}, []int{2, 5, 4, 6, 3, 1}, []int{4, 6, 5, 2, 1, 3},
		[]int{1, 3, 6, 2, 5, 4}, []int{1, 6, 5, 3, 2, 4}, []int{4, 6, 3, 5, 1, 2}, []int{3, 4, 6, 5, 2, 1}, []int{5, 1, 6, 2, 4, 3})
	preferences := s.computePairwisePreferences(votes, candidates)
	paths := s.computeStrongestPaths(preferences, candidates)
	require.Equal(t, cands(3, 5), s.findPotentialWinners(paths, candidates), "в профиле должна быть ничья 3 и 5")
	got, err := s.buildStrictOrder(candidates, preferences, paths, 1)
	require.NoError(t, err)
	assert.Equal(t, cands(5), got)
}

// Ничья за второе общее место решается тай-брейком по всему пулу: сильнейшие пути идут через уже избранного.
// Раньше тай-брейк получал только оставшихся кандидатов и падал с «too deep tie»
func TestBuildStrictOrderTieBreakUsesFullPool(t *testing.T) {
	s := &Schulze{}
	candidates := cands(1, 2, 3, 4, 5, 6)
	votes := ballots([]int{1, 3, 6, 5, 2, 4}, []int{1, 4, 5, 2, 6, 3}, []int{3, 2, 5, 6, 4, 1}, []int{2, 3, 6, 4, 1, 5},
		[]int{2, 3, 5, 1, 4, 6}, []int{6, 2, 4, 3, 1, 5}, []int{4, 6, 5, 3, 2, 1}, []int{2, 3, 5, 1, 6, 4},
		[]int{5, 2, 4, 3, 1, 6}, []int{4, 6, 3, 5, 1, 2}, []int{4, 5, 2, 3, 1, 6})
	preferences := s.computePairwisePreferences(votes, candidates)
	paths := s.computeStrongestPaths(preferences, candidates)
	got, err := s.buildStrictOrder(candidates, preferences, paths, 2)
	require.NoError(t, err)
	assert.Equal(t, cands(2, 4), got)
}

// Ничья, которую тай-брейк не разрешил: *TieError с местом, числом мест и уже избранными
func TestBuildStrictOrderUnresolvedTieReturnsTieError(t *testing.T) {
	s := &Schulze{}
	candidates := cands(3, 1, 2)
	// 3 — однозначно первый, 1 и 2 полностью равны
	preferences := s.computePairwisePreferences(ballots([]int{3, 1, 2}, []int{3, 2, 1}), candidates)
	paths := s.computeStrongestPaths(preferences, candidates)
	_, err := s.buildStrictOrder(candidates, preferences, paths, 2)
	var tie *TieError
	require.True(t, errors.As(err, &tie), "ожидалась *TieError, получено %v", err)
	assert.Equal(t, "", tie.Course)
	assert.Equal(t, 2, tie.Place)
	assert.Equal(t, 2, tie.Places)
	assert.Equal(t, cands(3), tie.Decided)
	assert.ElementsMatch(t, cands(1, 2), tie.Tied)
	assert.ElementsMatch(t, cands(1, 2), tie.Unresolved)
}

// Ничья на курсе: строка stage='tie' пишется, как раньше, и возвращается *TieError с курсом
func TestComputeResultsCourseTieReturnsTieError(t *testing.T) {
	ch := mocks.NewMockchain(gomock.NewController(t))
	var saved models.Result
	ch.EXPECT().AddResult(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r models.Result) error { saved = r; return nil })
	s := &Schulze{
		voteChain:          ch,
		candidatesByCourse: map[string][]models.Candidate{"1 бакалавриат": cands(1, 2)},
		votesByCourse:      map[string][]models.Vote{"1 бакалавриат": ballots([]int{1, 2}, []int{2, 1})},
	}
	err := s.ComputeResults(context.Background())
	var tie *TieError
	require.True(t, errors.As(err, &tie), "ожидалась *TieError, получено %v", err)
	assert.Equal(t, "1 бакалавриат", tie.Course)
	assert.ElementsMatch(t, cands(1, 2), tie.Unresolved)
	assert.Equal(t, "tie", saved.Stage)
	assert.ElementsMatch(t, []int{1, 2}, saved.WinnerCandidateID)
}

// Сбой записи результата курса больше не проглатывается
func TestComputeResultsReturnsAddResultError(t *testing.T) {
	ch := mocks.NewMockchain(gomock.NewController(t))
	boom := errors.New("db down")
	ch.EXPECT().AddResult(gomock.Any(), gomock.Any()).Return(boom)
	s := &Schulze{
		voteChain:          ch,
		candidatesByCourse: map[string][]models.Candidate{"1 бакалавриат": cands(1, 2)},
		votesByCourse:      map[string][]models.Vote{"1 бакалавриат": ballots([]int{1, 2})},
	}
	err := s.ComputeResults(context.Background())
	assert.ErrorIs(t, err, boom)
	var tie *TieError
	assert.False(t, errors.As(err, &tie))
}

// Все курсы посчитаны без ничьих — ошибки нет
func TestComputeResultsNoErrorWhenAllDecided(t *testing.T) {
	ch := mocks.NewMockchain(gomock.NewController(t))
	ch.EXPECT().AddResult(gomock.Any(), gomock.Any()).Return(nil).Times(2)
	s := &Schulze{
		voteChain: ch,
		candidatesByCourse: map[string][]models.Candidate{
			"1 бакалавриат": cands(1, 2),
			"2 бакалавриат": {{CandidateID: 3, Course: "2 бакалавриат"}},
		},
		votesByCourse: map[string][]models.Vote{
			"1 бакалавриат": ballots([]int{1, 2}, []int{1, 2}),
			"2 бакалавриат": ballots([]int{3}),
		},
	}
	assert.NoError(t, s.ComputeResults(context.Background()))
}

// Нет бюллетеней — ErrNoVotes, а не «успешный» подсчёт пустоты
func TestSetVotesNoVotes(t *testing.T) {
	ch := mocks.NewMockchain(gomock.NewController(t))
	ch.EXPECT().GetAllVotes(gomock.Any()).Return(nil, nil)
	s := &Schulze{voteChain: ch}
	assert.ErrorIs(t, s.SetVotes(), ErrNoVotes)
}

// Ничья не разрешена, но все оставшиеся кандидаты помещаются в оставшиеся места: избраны все, ошибки нет
func TestBuildStrictOrderUnresolvedTieButAllFit(t *testing.T) {
	s := &Schulze{}
	candidates := cands(3, 1, 2)
	preferences := s.computePairwisePreferences(ballots([]int{3, 1, 2}, []int{3, 2, 1}), candidates)
	paths := s.computeStrongestPaths(preferences, candidates)
	got, err := s.buildStrictOrder(candidates, preferences, paths, 3)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, 3, got[0].CandidateID)
	assert.ElementsMatch(t, cands(1, 2), got[1:])
}

// Кандидаты для подсчёта упорядочены по ID независимо от порядка строк в БД: исход тай-брейка воспроизводим
func TestSetCandidatesSortsByID(t *testing.T) {
	ch := mocks.NewMockchain(gomock.NewController(t))
	ch.EXPECT().GetAllEligibleCandidates(gomock.Any()).Return(cands(5, 1, 3), nil)
	s := &Schulze{voteChain: ch}
	require.NoError(t, s.SetCandidates())
	assert.Equal(t, cands(1, 3, 5), s.candidates)
}
