package schulze

import (
	"context"
	"fmt"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/config"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"

	"github.com/sirupsen/logrus"
)

// TODO какой метод построения рейтинга лучше? здесь или в calculate?
// Метод для вычисления глобального топ-N
func (s *Schulze) ComputeGlobalTop(ctx context.Context) error {
	allVotes := s.votes
	allCandidates := s.candidates

	// 1. Исключаем кандидатов, уже победивших в курсах
	commonCandidates, commonVotes, commonPlaces, err := s.excludeCourseWinners(ctx, allCandidates, allVotes)
	if err != nil {
		return fmt.Errorf("ComputeGlobalTop: %w", err)
	}
	if len(commonCandidates) == 0 {
		return fmt.Errorf("ComputeGlobalTop: no common candidates")
	}
	logrus.Debugf("commonCandidates: %v, commonVotes: %v, commonPlaces: %d", commonCandidates, commonVotes, commonPlaces)
	// 2. Вычисляем попарные предпочтения для оставшихся кандидатов
	commonPreferences := s.computePairwisePreferences(commonVotes, commonCandidates)
	logrus.Debugf("commonPreferences: %v", commonPreferences)
	// 3. Строим сильнейшие пути для оставшихся кандидатов
	commonStrongestPaths := s.computeStrongestPaths(commonPreferences, commonCandidates)
	logrus.Debugf("commonStrongestPaths: %v", commonStrongestPaths)

	// 6. Выбирам первых n кандидатов, решаем ничьи в случае необходимости
	globalTop, err := s.buildStrictOrder(commonCandidates, commonPreferences, commonStrongestPaths, commonPlaces)
	if err != nil {
		return fmt.Errorf("ComputeGlobalTop: %w", err)
	}
	logrus.Debugf("globalTop: %v", globalTop)

	// 6. Сохраняем глобальный топ-N
	var winnersIDs []int
	for _, candidate := range globalTop {
		winnersIDs = append(winnersIDs, candidate.CandidateID)
	}
	// TODO Проверить не слишком ли много/мало кандидатов на общие места

	result := models.Result{
		Course:            "Общие места",
		WinnerCandidateID: winnersIDs,
		Preferences:       commonPreferences,
		StrongestPaths:    commonStrongestPaths,
		Stage:             "common",
	}
	if err := s.voteChain.AddResult(ctx, result); err != nil {
		return fmt.Errorf("ComputeGlobalTop: %w", err)
	}

	return nil
}

// Метод для исключения кандидатов, победивших в курсах, из бюллетеней и списка
func (s *Schulze) excludeCourseWinners(ctx context.Context, allCandidates []models.Candidate, allvotes []models.Vote) ([]models.Candidate, []models.Vote, int, error) {
	// Исключаем победитилей по курсам из рейтинга общих вакантных мест
	excludedCandidateIDs := make(map[int]bool)
	results, err := s.voteChain.GetAllResults(ctx)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("excludeCourseWinners: %v", err)
	}
	for _, result := range results {
		if result.Stage == "common" {
			continue
		}
		if len(result.WinnerCandidateID) != 1 {
			return nil, nil, 0, fmt.Errorf("excludeCourseWinners: invalid number of winners for course %s: %d", result.Course, len(result.WinnerCandidateID))
		}
		winnerID := result.WinnerCandidateID[0]
		excludedCandidateIDs[winnerID] = true
	}
	commonPlaces := config.TotalPlaces - len(excludedCandidateIDs)

	commonCandidates := make([]models.Candidate, 0)
	for _, candidate := range allCandidates {
		if !excludedCandidateIDs[candidate.CandidateID] {
			commonCandidates = append(commonCandidates, candidate)
		}
	}

	// В бюллетенях общих мест оставляем только кандидатов общих мест: победители курсов исключены,
	// а снятые с выборов (is_eligible = false) и неизвестные ID в allCandidates не входят и тоже пропускаются
	commonIDs := make(map[int]bool, len(commonCandidates))
	for _, candidate := range commonCandidates {
		commonIDs[candidate.CandidateID] = true
	}
	coomonVotes := make([]models.Vote, 0)
	for _, vote := range allvotes {
		var filteredRankings []int
		for _, candidateID := range vote.CandidateRankings {
			if commonIDs[candidateID] {
				filteredRankings = append(filteredRankings, candidateID)
			}
		}
		if len(filteredRankings) > 0 {
			vote.CandidateRankings = filteredRankings
			coomonVotes = append(coomonVotes, vote)
		}

	}
	return commonCandidates, coomonVotes, commonPlaces, nil
}

// Расширенный метод для построения строгого порядка
func (s *Schulze) buildStrictOrder(candidates []models.Candidate, preferences, strongestPaths map[int]map[int]int, commonPlaces int) ([]models.Candidate, error) {
	strictOrder := make([]models.Candidate, 0)

	// Копируем список кандидатов, чтобы игнорировать уже ранжированных
	remainingCandidates := make([]models.Candidate, len(candidates))
	if l := copy(remainingCandidates, candidates); l != len(candidates) {
		return nil, fmt.Errorf("failed to copy slice")
	}

	places := commonPlaces
	// Пока есть оставшиеся кандидаты и места для ранжирования
	for len(remainingCandidates) > 0 && commonPlaces > 0 {
		// Шаг 1: Находим потенциальных победителей среди оставшихся кандидатов
		potentialWinners := s.findPotentialWinners(strongestPaths, remainingCandidates)
		// Шаг 2: Если несколько потенциальных победителей, разрешаем ничью.
		// Тай-брейку нужен весь пул candidates: матрицы путей посчитаны по всем кандидатам общих мест,
		// и сильнейшие пути между равными могут проходить через уже избранных
		if len(potentialWinners) > 1 {
			resolved, err := s.tieBreaker(potentialWinners, candidates, preferences, strongestPaths)
			if err != nil {
				return nil, fmt.Errorf("buildStrictOrder: %w", err)
			}
			// Тай-брейк не выбрал одного. Если все оставшиеся кандидаты помещаются в оставшиеся места,
			// избраны все и порядок между ними ни на что не влияет; иначе ничью решают вручную
			if len(resolved) > 1 && len(remainingCandidates) <= commonPlaces {
				logrus.Warnf("общие места: ничья за место %d между %s не разрешена, но все оставшиеся (%d) помещаются в оставшиеся места (%d) — избраны все", len(strictOrder)+1, candidateIDs(potentialWinners), len(remainingCandidates), commonPlaces)
				strictOrder = append(strictOrder, remainingCandidates...)
				break
			}
			if len(resolved) > 1 {
				return nil, &TieError{
					Place:      len(strictOrder) + 1,
					Places:     places,
					Decided:    append([]models.Candidate(nil), strictOrder...),
					Tied:       potentialWinners,
					Unresolved: resolved,
				}
			}
			logrus.Warnf("общие места: ничья за место %d между %s разрешена тай-брейком в пользу st%06d", len(strictOrder)+1, candidateIDs(potentialWinners), resolved[0].CandidateID)
			potentialWinners = resolved
		}

		// Шаг 3: Добавляем единственного победителя в начало строгого порядка
		strictOrder = append(strictOrder, potentialWinners[0])
		commonPlaces--

		// Шаг 4: Игнорируем победителя в дальнейших итерациях (убираем из оставшихся кандидатов)
		remainingCandidates = ignoreCandidate(remainingCandidates, potentialWinners[0].CandidateID)
	}
	return strictOrder, nil
}

// Вспомогательная функция для игнорирования кандидата по ID
func ignoreCandidate(candidates []models.Candidate, candidateID int) []models.Candidate {
	filtered := make([]models.Candidate, 0)
	for _, candidate := range candidates {
		if candidate.CandidateID != candidateID {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}
