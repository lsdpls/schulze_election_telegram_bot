package models

import (
	"database/sql"
	"time"
)

// Delegate представляет модель делегата
type Delegate struct {
	DelegateID int           `db:"delegate_id"` // Шестизначный код из st-email
	TelegramID sql.NullInt64 `db:"telegram_id"` // ID делегата в Telegram
	Name       string        `db:"name"`        // Имя делегата
	Group      string        `db:"group"`       // Уникальная группа делегата
	HasVoted   bool          `db:"has_voted"`   // Проголосовал ли делегат
}

// Candidate представляет модель кандидата
type Candidate struct {
	CandidateID int    `db:"candidate_id"` // Шестизначный код из st-email
	Name        string `db:"name"`         // Имя кандидата
	Course      string `db:"course"`       // Курс кандидата
	Description string `db:"description"`  // Описание кандидата
	IsEligible  bool   `db:"is_eligible"`  // Допущен ли кандидат до выборов
}

// Vote представляет модель голосования
type Vote struct {
	ID                int       `db:"id"`                 // Уникальный идентификатор голосования
	DelegateID        int       `db:"delegate_id"`        // ID делегата
	CandidateRankings []int     `db:"candidate_rankings"` // Ранжирование кандидатов
	CreatedAt         time.Time `db:"created_at"`         // Время создания голосования
}

// Result представляет модель результатов
type Result struct {
	ID                int                 `db:"id"`                  // Уникальный идентификатор результатов
	Course            string              `db:"course"`              // Вакантное место
	WinnerCandidateID []int               `db:"winner_candidate_id"` // ID победителя
	Preferences       map[int]map[int]int `db:"preferences"`         // Парные предпочтения
	StrongestPaths    map[int]map[int]int `db:"strongest_paths"`     // Сильнейшие пути
	Stage             string              `db:"stage"`               // Состояние результатов (на каком этапе получены результаты)
}
