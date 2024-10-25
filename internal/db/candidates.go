package db

import (
	"context"
	"fmt"
	"vote_system/internal/models"

	"github.com/jackc/pgx/v5"
)

// Добавление кандидата
func (s *Storage) AddCandidate(ctx context.Context, tx pgx.Tx, candidate models.Candidate) error {
	_, err := tx.Exec(ctx,
		"INSERT INTO candidates (candidate_id, name, course, description, is_eligible) VALUES ($1, $2, $3, $4, $5)",
		candidate.CandidateID, candidate.Name, candidate.Course, candidate.Description, candidate.IsEligible)
	if err != nil {
		return fmt.Errorf("db.AddCandidate: insert failed: %w", err)
	}

	return nil
}

// Получение кандидата по ID кандидата
func (s *Storage) GetCandidateByCandidateID(ctx context.Context, tx pgx.Tx, candidateID int) (*models.Candidate, error) {
	var candidate models.Candidate
	err := tx.QueryRow(ctx, "SELECT * FROM candidates WHERE candidate_id = $1", candidateID).Scan(
		&candidate.CandidateID,
		&candidate.Name,
		&candidate.Course,
		&candidate.Description,
		&candidate.IsEligible,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("db.GetCandidateByCandidateID: %w", err)
	}
	return &candidate, nil
}

func (s *Storage) GetAllCandidates(ctx context.Context, tx pgx.Tx) ([]models.Candidate, error) {
	rows, err := tx.Query(ctx, "SELECT * FROM candidates")
	if err != nil {
		return nil, fmt.Errorf("db.GetAllCandidates: %w", err)
	}
	defer rows.Close()
	var candidates []models.Candidate
	for rows.Next() {
		var candidate models.Candidate
		if err := rows.Scan(
			&candidate.CandidateID,
			&candidate.Name,
			&candidate.Course,
			&candidate.Description,
			&candidate.IsEligible,
		); err != nil {
			return nil, fmt.Errorf("db.GetAllCandidates: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("db.GetAllCandidates: %w", err)
	}
	return candidates, nil
}
func (s *Storage) GetAllEligibleCandidates(ctx context.Context, tx pgx.Tx) ([]models.Candidate, error) {
	rows, err := tx.Query(ctx, "SELECT * FROM candidates WHERE is_eligible = TRUE")
	if err != nil {
		return nil, fmt.Errorf("db.GetAllEligibleCandidates: %w", err)
	}
	defer rows.Close()
	var candidates []models.Candidate
	for rows.Next() {
		var candidate models.Candidate
		if err := rows.Scan(
			&candidate.CandidateID,
			&candidate.Name,
			&candidate.Course,
			&candidate.Description,
			&candidate.IsEligible,
		); err != nil {
			return nil, fmt.Errorf("db.GetAllEligibleCandidates: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("db.GetAllEligibleCandidates: %w", err)
	}
	return candidates, nil
}

// Обновление кандидата
func (s *Storage) UpdateCandidate(ctx context.Context, tx pgx.Tx, candidate models.Candidate) error {
	_, err := tx.Exec(ctx,
		"UPDATE candidates SET name = $1, course = $2, description = $3, is_eligible = $4 WHERE candidate_id = $5",
		candidate.Name, candidate.Course, candidate.Description, candidate.IsEligible, candidate.CandidateID)
	if err != nil {
		return fmt.Errorf("db.UpdateCandidate: %w", err)
	}

	return nil
}

// Удаление кандидата
func (s *Storage) DeleteCandidate(ctx context.Context, tx pgx.Tx, candidateID int) error {
	_, err := tx.Exec(ctx, "DELETE FROM candidates WHERE candidate_id = $1", candidateID)
	if err != nil {
		return fmt.Errorf("db.DeleteCandidate: %w", err)
	}

	return nil
}
