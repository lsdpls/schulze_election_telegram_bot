package schulze

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/sirupsen/logrus"
)

// Метод для получения строкового представления таблиц парных предпочтений и сильнейших путей для каждого курса
func (s *Schulze) GetResultsString() (string, error) {
	var builder strings.Builder
	results, err := s.voteChain.GetAllResults(context.Background())
	if err != nil {
		return "", fmt.Errorf("failed to get results: %w", err)
	}
	if len(results) == 0 {
		return "", fmt.Errorf("no results found")
	}

	for _, result := range results {
		candidateOrder := make([]int, 0, len(result.Preferences))
		for i := range result.Preferences {
			candidateOrder = append(candidateOrder, i)
		}

		slices.Sort(candidateOrder)
		builder.WriteString(fmt.Sprintf("<b>Курс: %s</b>\n", result.Course))
		builder.WriteString("<b>Победители:")
		for i, winnerID := range result.WinnerCandidateID {
			winnerName, err := s.voteChain.GetCandidateByCandidateID(context.Background(), result.WinnerCandidateID[i])
			if err != nil {
				return "", fmt.Errorf("failed to get winner name: %w", err)
			}
			builder.WriteString(fmt.Sprintf(" st%s %s;", idtos(winnerID), winnerName.Name))
		}
		builder.WriteString("</b>\n")
		builder.WriteString(s.preferencesToString(result.Preferences, candidateOrder))
		builder.WriteString(s.strongestPathsToString(result.StrongestPaths, candidateOrder))

		builder.WriteString("—————\n")

	}
	logrus.Info(builder.String())
	return builder.String(), nil
}

func (s *Schulze) preferencesToString(preferences map[int]map[int]int, order []int) string {
	var builder strings.Builder
	builder.WriteString("Таблица парных предпочтений:\n")
	builder.WriteString(fmt.Sprintf("%-10s", "——   "))
	for _, candidateID := range order {
		builder.WriteString(fmt.Sprintf("%-10s", idtos(candidateID)))
	}
	builder.WriteString("\n")
	for _, candidateID1 := range order {
		builder.WriteString(fmt.Sprintf("%-10s", idtos(candidateID1)))
		for _, candidateID2 := range order {
			if candidateID1 != candidateID2 {
				builder.WriteString(fmt.Sprintf("%-15d", preferences[candidateID1][candidateID2]))
			} else {
				builder.WriteString(fmt.Sprintf("%-15s", "—"))
			}
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
	return builder.String()
}

func (s *Schulze) strongestPathsToString(strongestPaths map[int]map[int]int, order []int) string {
	var builder strings.Builder
	builder.WriteString("Таблица сильнейших путей:\n")
	builder.WriteString(fmt.Sprintf("%-10s", "——   "))
	for _, candidateID := range order {
		builder.WriteString(fmt.Sprintf("%-10s", idtos(candidateID)))
	}
	builder.WriteString("\n")
	for _, candidateID1 := range order {
		builder.WriteString(fmt.Sprintf("%-10s", idtos(candidateID1)))
		for _, candidateID2 := range order {
			if candidateID1 != candidateID2 {
				builder.WriteString(fmt.Sprintf("%-15d", strongestPaths[candidateID1][candidateID2]))
			} else {
				builder.WriteString(fmt.Sprintf("%-15s", "—"))
			}
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
	return builder.String()
}

func idtos(number int) string {
	numberStr := fmt.Sprintf("%d", number)
	re := regexp.MustCompile(`^\d{6}$`)
	if re.MatchString(numberStr) {
		return numberStr
	}
	// Add leading zeros
	for len(numberStr) < 6 {
		numberStr = "0" + numberStr
	}
	return numberStr
}
