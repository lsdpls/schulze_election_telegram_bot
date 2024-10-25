package schulze

import (
	"context"
	"fmt"
	"vote_system/internal/models"

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
	commonPlaces := 10 - len(excludedCandidateIDs)

	commonCandidates := make([]models.Candidate, 0)
	for _, candidate := range allCandidates {
		if !excludedCandidateIDs[candidate.CandidateID] {
			commonCandidates = append(commonCandidates, candidate)
		}
	}

	coomonVotes := make([]models.Vote, 0)
	for _, vote := range allvotes {
		var filteredRankings []int
		for _, candidateID := range vote.CandidateRankings {
			if !excludedCandidateIDs[candidateID] {
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

	// Пока есть оставшиеся кандидаты и места для ранжирования
	for len(remainingCandidates) > 0 && commonPlaces > 0 {
		// Шаг 1: Находим потенциальных победителей среди оставшихся кандидатов
		potentialWinners := s.findPotentialWinners(strongestPaths, remainingCandidates)
		// Шаг 2: Если несколько потенциальных победителей, разрешаем ничью
		if len(potentialWinners) > 1 {
			potentialWinners, err := s.tieBreaker(potentialWinners, remainingCandidates, preferences, strongestPaths)
			if err != nil {
				return nil, err // TODO всегда nil
			}
			// Если ничья не разрешена, то у нас слишком глубокая ничья
			if len(potentialWinners) > 1 {
				return nil, fmt.Errorf("нет строгого порядка: слишком глубокая ничья")
			}
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
