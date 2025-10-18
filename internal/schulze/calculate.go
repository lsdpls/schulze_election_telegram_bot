package schulze

import (
	"context"
	"fmt"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"

	"github.com/sirupsen/logrus"
)

// Метод для вычисления результатов голосования по методу Шульце
func (s *Schulze) ComputeResults(ctx context.Context) error {
	for course, votes := range s.votesByCourse {
		candidates := s.candidatesByCourse[course]
		var result models.Result
		// candidatesIDs := make([]int, len(candidates))
		// for i, candidate := range candidates {
		// 	candidatesIDs[i] = candidate.CandidateID
		// }

		// 1. Подсчет попарных предпочтений
		preferences := s.computePairwisePreferences(votes, candidates)
		// 2. Построение сильнейших путей
		strongestPaths := s.computeStrongestPaths(preferences, candidates)
		// 3. Поиск победителя
		potentialWinners := s.findPotentialWinners(strongestPaths, candidates)

		if len(potentialWinners) < 1 {
			logrus.Errorf("no winners for course %s", course)
			// TODO отправить это куда нужно
			continue
		}
		// 4. Сохранение результатов
		if len(potentialWinners) == 1 {
			result = models.Result{
				Course:            course,
				WinnerCandidateID: []int{potentialWinners[0].CandidateID},
				Preferences:       preferences,
				StrongestPaths:    strongestPaths,
				Stage:             "absolute",
			}
			// TODO отправить это куда нужно
			if err := s.voteChain.AddResult(ctx, result); err != nil {
				logrus.Errorf("cant AddResult for %s: %v", course, err)
			}
			continue
		}

		// 5. Если ничья (нет однозначного победителя)
		potentialWinners, err := s.tieBreaker(potentialWinners, candidates, preferences, strongestPaths)
		if err != nil {
			// TODO Всегда nil в текущей реализации
			continue
		}

		if len(potentialWinners) == 1 {
			result = models.Result{
				Course:            course,
				WinnerCandidateID: []int{potentialWinners[0].CandidateID},
				Preferences:       preferences,
				StrongestPaths:    strongestPaths,
				Stage:             "tie-breaker",
			}
			// TODO отправить это куда нужно
			if err := s.voteChain.AddResult(ctx, result); err != nil {
				logrus.Errorf("cant AddResult for %s: %v", course, err)
			}
			continue
		}
		var winnersIDs []int
		for _, candidate := range potentialWinners {
			winnersIDs = append(winnersIDs, candidate.CandidateID)
		}
		result = models.Result{
			Course:            course,
			WinnerCandidateID: winnersIDs,
			Preferences:       preferences,
			StrongestPaths:    strongestPaths,
			Stage:             "tie",
		}
		if err := s.voteChain.AddResult(ctx, result); err != nil {
			logrus.Errorf("cant AddResult for %s: %v", course, err)
		}
	}

	return nil
}

// TODO унифицировать итерации по слайсам: то if ==, то [i+1], то if !=
// TODO везде итерироваться по ID, а не копировать огромные структуры
// Шаг 1: Метод для подсчета попарных предпочтений
func (s *Schulze) computePairwisePreferences(votes []models.Vote, candidates []models.Candidate) map[int]map[int]int {
	// Подготовим карту для подсчета попарных предпочтений
	pairwisePreferences := make(map[int]map[int]int)
	for _, c1 := range candidates {
		pairwisePreferences[c1.CandidateID] = make(map[int]int)
		for _, c2 := range candidates {
			if c1.CandidateID != c2.CandidateID {
				pairwisePreferences[c1.CandidateID][c2.CandidateID] = 0
			}
		}
	}
	// Подсчёт попарных предпочтений на основе ранжировок
	for _, vote := range votes {
		for i, candidate1 := range vote.CandidateRankings {
			for _, candidate2 := range vote.CandidateRankings[i+1:] {
				pairwisePreferences[candidate1][candidate2]++
			}
		}
	}
	return pairwisePreferences
}

// Шаг 2: Построение сильнейших путей
func (s *Schulze) computeStrongestPaths(preferences map[int]map[int]int, candidates []models.Candidate) map[int]map[int]int {
	// Инициализация сильнейших путей
	strongestPaths := make(map[int]map[int]int)
	for _, c1 := range candidates {
		strongestPaths[c1.CandidateID] = make(map[int]int)
		for _, c2 := range candidates {
			if c1.CandidateID != c2.CandidateID {
				// Устанавливаем начальную силу пути только если d[A][B] > d[B][A]
				if preferences[c1.CandidateID][c2.CandidateID] > preferences[c2.CandidateID][c1.CandidateID] {
					strongestPaths[c1.CandidateID][c2.CandidateID] = preferences[c1.CandidateID][c2.CandidateID]
				} else {
					// Если нет предпочтения в пользу c1 перед c2, начальная сила пути равна 0
					strongestPaths[c1.CandidateID][c2.CandidateID] = 0
				}
			}
		}
	}
	// Алгоритм Флойда-Уоршелла для нахождения сильнейших путей
	for _, i := range candidates {
		for _, j := range candidates {
			if i.CandidateID != j.CandidateID {
				for _, k := range candidates {
					if i.CandidateID != k.CandidateID && j.CandidateID != k.CandidateID {
						// Проверяем, существует ли путь j -> i и i -> k
						if strongestPaths[j.CandidateID][i.CandidateID] > 0 && strongestPaths[i.CandidateID][k.CandidateID] > 0 {
							currentPath := strongestPaths[j.CandidateID][k.CandidateID]
							potentialPath := min(strongestPaths[j.CandidateID][i.CandidateID], strongestPaths[i.CandidateID][k.CandidateID])
							if potentialPath > currentPath {
								strongestPaths[j.CandidateID][k.CandidateID] = potentialPath
							}
						}
					}
				}
			}
		}
	}
	return strongestPaths
}

// Шаг 3: Нахождение потенциальных победителей
func (s *Schulze) findPotentialWinners(strongestPaths map[int]map[int]int, candidates []models.Candidate) []models.Candidate {
	potentialWinners := make([]models.Candidate, 0)
	// Проверяем, есть ли однозначный победитель
	for _, candidate := range candidates {
		isPotentialWinner := true
		for _, opponent := range candidates {
			if candidate.CandidateID != opponent.CandidateID {
				logrus.Debugf("candidate: %d, opponent: %d, strongestPaths[candidate][opponent]: %d, strongestPaths[opponent][candidate]: %d", candidate.CandidateID, opponent.CandidateID, strongestPaths[candidate.CandidateID][opponent.CandidateID], strongestPaths[opponent.CandidateID][candidate.CandidateID])
				if strongestPaths[opponent.CandidateID][candidate.CandidateID] > strongestPaths[candidate.CandidateID][opponent.CandidateID] {
					isPotentialWinner = false
					break
				}
			}
		}
		if isPotentialWinner {
			potentialWinners = append(potentialWinners, candidate)
			logrus.Debugf("potentialWinner: %v", potentialWinners)
		}
	}
	return potentialWinners
}

// Поиск всех слабейших звеньев на сильнейшем пути от A к B
func (s *Schulze) findAllWeakestEdges(preferences, strongestPaths map[int]map[int]int, start, end int, candidates []models.Candidate) [][]int {
	var weakestEdges [][]int
	visited := make(map[int]bool)
	strongestPathStrength := strongestPaths[start][end]
	edgeSet := make(map[string]struct{})
	EdgeKey := func(from, to int) string {
		return fmt.Sprintf("%d-%d", from, to)
	}

	// Рекурсивный поиск всех путей от start к end
	var dfs func(current int, path []int)
	dfs = func(current int, path []int) {
		// Если достигли конечного кандидата (to)
		if current == end {
			// Находим минимальное ребро в пути
			minEdgeStrength := preferences[path[0]][path[1]] // Инициализируем первым ребром
			for i := 1; i < len(path)-1; i++ {
				edgeStrength := preferences[path[i]][path[i+1]]
				if edgeStrength < minEdgeStrength {
					minEdgeStrength = edgeStrength
				}
			}
			// Если сила пути != искомой силе пути, то пропускаем запись ребер этого пути
			if minEdgeStrength != strongestPathStrength {
				return
			}
			// Проверяем все ребра в пути на соответствие силе сильнейшего пути
			for i := 0; i < len(path)-1; i++ {
				c1, c2 := path[i], path[i+1]
				if preferences[c1][c2] == strongestPathStrength {
					if _, exist := edgeSet[EdgeKey(c1, c2)]; !exist {
						weakestEdges = append(weakestEdges, []int{c1, c2})
						edgeSet[EdgeKey(c1, c2)] = struct{}{} // Отмечаем ребро как добавленное
					}
				}
			}
			return
		}

		// Отмечаем узел как посещённый
		visited[current] = true

		// Проходим по соседним кандидатам, используя preferences для проверки предпочтений
		for _, next := range candidates {
			// Двигаемся только в сторону, где предпочтение current > next
			if !visited[next.CandidateID] && preferences[current][next.CandidateID] > preferences[next.CandidateID][current] {
				newPath := make([]int, len(path))                        // Копируем текущий путь, чтобы передать в рекурсию новую версию
				copy(newPath, path)                                      // Копируем содержимое старого пути в новый
				dfs(next.CandidateID, append(newPath, next.CandidateID)) // Добавляем нового кандидата в путь
			}
		}

		// Сбрасываем состояние текущего узла как непосещённый для других путей
		visited[current] = false
	}

	// Запускаем поиск с кандидата from (A)
	dfs(start, []int{start})

	return weakestEdges
}

// Шаг 4: Решение ничьей
func (s *Schulze) tieBreaker(potentialWinners []models.Candidate, candidates []models.Candidate, preferences, strongestPaths map[int]map[int]int) ([]models.Candidate, error) {
	// https://arxiv.org/pdf/1804.02973
	// для A <=> B решаем ничью и только для них
	// удаляем общие слабейшие звенья пока можем
	// (если не можем - пока забываем про A-B)
	// записываем новые данные в newStrongestPath
	// допустим A>B, тогда убираем B из потенциальных победителей
	// решаем ничью для следующей пары победителей
	// Рано или поздно получим единственного транзитивного победителя
	// Если не получим, то мега ничья

	// Создаем временную переменную
	tmpPotentialWinners := make([]models.Candidate, len(potentialWinners))
	if l := copy(tmpPotentialWinners, potentialWinners); l != len(potentialWinners) {
		return nil, fmt.Errorf("failed to copy slice")
	}

	for len(tmpPotentialWinners) > 1 {
		// Берем первый двух кандидатов, т.к. порядок не имеет значения на разрешение ничьей и построение тразитивновного неравества (5.2.5. https://arxiv.org/pdf/1804.02973)
		foundWinner := false
		// Перебираем все пары кандидатов из потенциальных победителей
		for i := 0; i < len(tmpPotentialWinners)-1; i++ {
			for j := i + 1; j < len(tmpPotentialWinners); j++ {
				c1 := tmpPotentialWinners[i]
				c2 := tmpPotentialWinners[j]
				logrus.Debugf("c1: %d, c2: %d\n", c1.CandidateID, c2.CandidateID)

				// Копируем мапы чтоб случайно не изменить их
				tmpStrongestPaths := copyMapOfMap(strongestPaths, candidates)
				tmpPreferences := copyMapOfMap(preferences, candidates)
				logrus.Debugf("tmpStrongestPaths: %v, tmpPreferences: %v\n", tmpStrongestPaths, tmpPreferences)

				for {
					// Находим все слабейшие звенья между A и B
					weakestEdgesAB := s.findAllWeakestEdges(tmpPreferences, tmpStrongestPaths, c1.CandidateID, c2.CandidateID, candidates)
					weakestEdgesBA := s.findAllWeakestEdges(tmpPreferences, tmpStrongestPaths, c2.CandidateID, c1.CandidateID, candidates)

					// Выходим из цикла если не осталось общих ребер
					equalLinks := s.findEqualLinks(weakestEdgesAB, weakestEdgesBA)
					logrus.Debugf("weakestEdgesAB: %v, weakestEdgesBA: %v, equalLinks: %v\n", weakestEdgesAB, weakestEdgesBA, equalLinks)
					if equalLinks == nil {
						break
					}

					// TODO мы сразу все похожие удаляем?
					// Проходим по совпадающим звеньям
					// for _, equalLink := range equalLinks {
					// 	tmpPreferences[equalLink[0]][equalLink[1]] = 0
					// 	tmpStrongestPaths[equalLink[0]][equalLink[1]] = 0
					// }

					// Берем первое попавшееся одинаковое звено
					equalLink := equalLinks[0]
					tmpPreferences[equalLink[0]][equalLink[1]] = 0
					tmpStrongestPaths[equalLink[0]][equalLink[1]] = 0

					tmpStrongestPaths = s.computeStrongestPaths(tmpPreferences, candidates)
					if tmpStrongestPaths[c1.CandidateID][c2.CandidateID] > tmpStrongestPaths[c2.CandidateID][c1.CandidateID] {
						// c1 выигрывает -> исключаем c2
						tmpPotentialWinners = removeCandidate(tmpPotentialWinners, j)
						logrus.Debugf("tmpPotentialWinners: %v\n", tmpPotentialWinners)
						foundWinner = true
						break
					} else if tmpStrongestPaths[c1.CandidateID][c2.CandidateID] < tmpStrongestPaths[c2.CandidateID][c1.CandidateID] {
						// c2 выигрывает -> исключаем c1
						tmpPotentialWinners = removeCandidate(tmpPotentialWinners, i)
						foundWinner = true
						break
					}
				}
				// Если победитель найден, выходим из цикла по парам
				if foundWinner {
					break
				}
			}
			// Если победитель найден, начинаем новый цикл с обновленным списком
			if foundWinner {
				break
			}
		}
		// Если не удалось разрешить ничью совсем, то выходим из попыток
		if !foundWinner {
			break
		}
	}
	return tmpPotentialWinners, nil
}

// Вспомогательная функция для поиска одинаковых звеньев
func (s *Schulze) findEqualLinks(weakestEdgesAB, weakestEdgesBA [][]int) [][]int {
	var equalLinks [][]int
	for _, linkAB := range weakestEdgesAB {
		for _, linkBA := range weakestEdgesBA {
			if linkAB[0] == linkBA[0] && linkAB[1] == linkBA[1] {
				equalLinks = append(equalLinks, linkAB)
			}
		}
	}
	return equalLinks
}

// Вспомогательная функция для копирования предпочтений
func copyMapOfMap(original map[int]map[int]int, candidates []models.Candidate) map[int]map[int]int {
	tmpPreferences := make(map[int]map[int]int)
	for _, j := range candidates {
		tmpPreferences[j.CandidateID] = make(map[int]int)
		for _, k := range candidates {
			if j.CandidateID != k.CandidateID {
				tmpPreferences[j.CandidateID][k.CandidateID] = original[j.CandidateID][k.CandidateID]
			}
		}
	}
	return tmpPreferences
}

// Вспомогательная функция для удаления кандидата из списка победителей
func removeCandidate(candidates []models.Candidate, index int) []models.Candidate {
	return append(candidates[:index], candidates[index+1:]...)
}
