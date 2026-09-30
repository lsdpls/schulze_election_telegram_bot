package schulze

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"
)

// ErrNoVotes — в базе нет ни одного бюллетеня: считать нечего
var ErrNoVotes = errors.New("нет ни одного бюллетеня")

// TieError — ничья, которую встроенный тай-брейк (tieBreaker) не разрешил. Победителя определяют вручную.
// Для курса строка results со stage='tie' записывается как раньше; для общих мест строка не пишется.
type TieError struct {
	Course     string             // курс; пусто — ничья в общих местах
	Place      int                // общие места: номер места, за которое ничья (с 1)
	Places     int                // общие места: сколько всего общих мест
	Decided    []models.Candidate // общие места: кандидаты, избранные до ничьей, по порядку
	Tied       []models.Candidate // кандидаты, равные по методу Шульце (никто из них не сильнее остальных)
	Unresolved []models.Candidate // кто остался равным после тай-брейка (подмножество Tied)
}

func (e *TieError) Error() string {
	if e.Course != "" {
		return fmt.Sprintf("неразрешённая ничья на курсе %s: %s", e.Course, candidateIDs(e.Unresolved))
	}
	return fmt.Sprintf("неразрешённая ничья в общих местах за место %d из %d: %s", e.Place, e.Places, candidateIDs(e.Unresolved))
}

func candidateIDs(candidates []models.Candidate) string {
	ids := make([]string, len(candidates))
	for i, c := range candidates {
		ids[i] = fmt.Sprintf("st%06d", c.CandidateID)
	}
	return strings.Join(ids, ", ")
}
