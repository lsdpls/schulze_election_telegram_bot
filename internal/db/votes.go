package db

import (
	"context"
	"fmt"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"

	"github.com/jackc/pgx/v5"
)

// Добавление голоса
func (s *Storage) AddVote(ctx context.Context, tx pgx.Tx, vote models.Vote) error {
	_, err := tx.Exec(ctx,
		"INSERT INTO votes (delegate_id, candidate_rankings, created_at) VALUES ($1, $2, $3)",
		vote.DelegateID, vote.CandidateRankings, vote.CreatedAt)
	if err != nil {
		return fmt.Errorf("AddVote: insert failed: %w", err)
	}

	return nil
}

// Получение голоса по ID делегата
func (s *Storage) GetVoteByDelegateID(ctx context.Context, tx pgx.Tx, delegateID int) (*models.Vote, error) {
	var vote models.Vote
	err := tx.QueryRow(ctx, "SELECT * FROM votes WHERE delegate_id = $1", delegateID).Scan(
		&vote.ID,
		&vote.DelegateID,
		&vote.CandidateRankings,
		&vote.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetVoteByDelegateID: query failed: %w", err)
	}
	return &vote, nil
}

// Получение всех голосов
func (s *Storage) GetAllVotes(ctx context.Context, tx pgx.Tx) ([]models.Vote, error) {
	rows, err := tx.Query(ctx, "SELECT * FROM votes")
	if err != nil {
		return nil, fmt.Errorf("GetAllVotes: query failed: %w", err)
	}
	defer rows.Close()

	var votes []models.Vote
	for rows.Next() {
		var vote models.Vote
		err := rows.Scan(
			&vote.ID,
			&vote.DelegateID,
			&vote.CandidateRankings,
			&vote.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("GetAllVotes: scan failed: %w", err)
		}
		votes = append(votes, vote)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetAllVotes: rows error: %w", err)
	}
	return votes, nil
}

// Обновление голоса
func (s *Storage) UpdateVote(ctx context.Context, tx pgx.Tx, vote models.Vote) error {
	_, err := tx.Exec(ctx,
		"UPDATE votes SET candidate_rankings = $1, created_at = $2 WHERE delegate_id = $3",
		vote.CandidateRankings, vote.CreatedAt, vote.DelegateID)
	if err != nil {
		return fmt.Errorf("UpdateVote: update failed: %w", err)
	}

	return nil
}

// Удаление голоса
func (s *Storage) DeleteVote(ctx context.Context, tx pgx.Tx, voteID int) error {
	_, err := tx.Exec(ctx, "DELETE FROM votes WHERE id = $1", voteID)
	if err != nil {
		return fmt.Errorf("DeleteVote: delete failed: %w", err)
	}

	return nil
}
