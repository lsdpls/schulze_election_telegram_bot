package db

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"

	"github.com/jackc/pgx/v5"
)

func (s *Storage) AddResult(ctx context.Context, tx pgx.Tx, result models.Result) error {
	// Преобразование слайсов в JSON
	preferencesJSON, err := json.Marshal(result.Preferences)
	if err != nil {
		return fmt.Errorf("AddResult: marshal preferences failed: %w", err)
	}
	strongestPathsJSON, err := json.Marshal(result.StrongestPaths)
	if err != nil {
		return fmt.Errorf("AddResult: marshal strongest paths failed: %w", err)
	}

	// Вставка результатов в базу данных
	_, err = tx.Exec(ctx,
		"INSERT INTO results (course, winner_candidate_id, preferences, strongest_paths, stage) VALUES ($1, $2, $3, $4, $5)",
		result.Course, result.WinnerCandidateID, preferencesJSON, strongestPathsJSON, result.Stage)
	if err != nil {
		return fmt.Errorf("AddResult: insert failed: %w", err)
	}

	return nil
}
func (s *Storage) GetResultByCourse(ctx context.Context, tx pgx.Tx, course string) (*models.Result, error) {
	var result models.Result
	var preferencesJSON, strongestPathsJSON string
	err := tx.QueryRow(ctx, "SELECT * FROM results WHERE course = $1", course).Scan(
		&result.ID,
		&result.Course,
		&result.WinnerCandidateID,
		&preferencesJSON,    // Считываем JSON как строку
		&strongestPathsJSON, // Считываем JSON как строку
		&result.Stage,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetResultByCourse: query failed: %w", err)
	}
	// Десериализация JSON
	if err := json.Unmarshal([]byte(preferencesJSON), &result.Preferences); err != nil {
		return nil, fmt.Errorf("GetResultByCourse: unmarshal preferences failed: %w", err)
	}
	if err := json.Unmarshal([]byte(strongestPathsJSON), &result.StrongestPaths); err != nil {
		return nil, fmt.Errorf("GetResultByCourse: unmarshal strongest paths failed: %w", err)
	}
	return &result, nil
}

func (s *Storage) GetAllResults(ctx context.Context, tx pgx.Tx) ([]models.Result, error) {
	rows, err := tx.Query(ctx, "SELECT * FROM results")
	if err != nil {
		return nil, fmt.Errorf("GetAllResults: query failed: %w", err)
	}
	defer rows.Close()

	var results []models.Result
	for rows.Next() {
		var result models.Result
		var preferencesJSON, strongestPathsJSON string
		err := rows.Scan(
			&result.ID,
			&result.Course,
			&result.WinnerCandidateID,
			&preferencesJSON,    // Считываем JSON как строку
			&strongestPathsJSON, // Считываем JSON как строку
			&result.Stage,
		)
		if err != nil {
			return nil, fmt.Errorf("GetAllResults: scan failed: %w", err)
		}
		// Десериализация JSON
		if err := json.Unmarshal([]byte(preferencesJSON), &result.Preferences); err != nil {
			return nil, fmt.Errorf("GetAllResults: unmarshal preferences failed: %w", err)
		}
		if err := json.Unmarshal([]byte(strongestPathsJSON), &result.StrongestPaths); err != nil {
			return nil, fmt.Errorf("GetAllResults: unmarshal strongest paths failed: %w", err)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetAllResults: rows error: %w", err)
	}
	return results, nil
}

func (s *Storage) UpdateResult(ctx context.Context, tx pgx.Tx, result models.Result) error {
	// Преобразование слайсов в JSON
	preferencesJSON, err := json.Marshal(result.Preferences)
	if err != nil {
		return fmt.Errorf("UpdateResult: marshal preferences failed: %w", err)
	}
	strongestPathsJSON, err := json.Marshal(result.StrongestPaths)
	if err != nil {
		return fmt.Errorf("UpdateResult: marshal strongest paths failed: %w", err)
	}

	// Обновление результатов в базе данных
	_, err = tx.Exec(ctx,
		"UPDATE results SET winner_candidate_id = $1, preferences = $2, strongest_paths = $3, stage = $4 WHERE course = $5",
		result.WinnerCandidateID, preferencesJSON, strongestPathsJSON, result.Stage, result.Course)
	if err != nil {
		return fmt.Errorf("UpdateResult: update failed: %w", err)
	}

	return nil
}

// Удаление результата
func (s *Storage) DeleteResult(ctx context.Context, tx pgx.Tx, resultID int) error {
	_, err := tx.Exec(ctx, "DELETE FROM results WHERE id = $1", resultID)
	if err != nil {
		return fmt.Errorf("DeleteResult: delete failed: %w", err)
	}

	return nil
}
