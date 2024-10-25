package db

import (
	"context"
	"fmt"
	"vote_system/internal/models"

	"github.com/jackc/pgx/v5"
)

// Добавление делегата
func (s *Storage) AddDelegate(ctx context.Context, tx pgx.Tx, delegate models.Delegate) error {
	_, err := tx.Exec(ctx,
		"INSERT INTO delegates (delegate_id, telegram_id, name, delegate_group) VALUES ($1, $2, $3, $4)",
		delegate.DelegateID, delegate.TelegramID, delegate.Name, delegate.Group)
	if err != nil {
		return fmt.Errorf("db.AddDelegate: %w", err)
	}

	return nil
}

// Получение делегата по ID делегата
func (s *Storage) GetDelegateByDelegateID(ctx context.Context, tx pgx.Tx, delegateID int) (*models.Delegate, error) {
	var delegate models.Delegate
	err := tx.QueryRow(ctx, "SELECT * FROM delegates WHERE delegate_id = $1", delegateID).Scan(
		&delegate.DelegateID,
		&delegate.TelegramID,
		&delegate.Name,
		&delegate.Group,
		&delegate.HasVoted,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("db.GetDelegateByDelegateID: %w", err)
	}
	return &delegate, nil
}

// Получение делегата по Telegram ID
func (s *Storage) GetDelegateByTelegramID(ctx context.Context, tx pgx.Tx, telegramID int64) (*models.Delegate, error) {
	var delegate models.Delegate
	err := tx.QueryRow(ctx, "SELECT * FROM delegates WHERE telegram_id = $1", telegramID).Scan(
		&delegate.DelegateID,
		&delegate.TelegramID,
		&delegate.Name,
		&delegate.Group,
		&delegate.HasVoted,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("db.GetDelegateByTelegramID: %w", err)
	}
	return &delegate, nil
}
func (s *Storage) GetAllDelegates(ctx context.Context, tx pgx.Tx) ([]models.Delegate, error) {
	rows, err := tx.Query(ctx, "SELECT * FROM delegates")
	if err != nil {
		return nil, fmt.Errorf("db.GetDelegates: %w", err)
	}
	defer rows.Close()

	var delegates []models.Delegate
	for rows.Next() {
		var delegate models.Delegate
		if err := rows.Scan(
			&delegate.DelegateID,
			&delegate.TelegramID,
			&delegate.Name,
			&delegate.Group,
			&delegate.HasVoted,
		); err != nil {
			return nil, fmt.Errorf("db.GetDelegates: %w", err)
		}
		delegates = append(delegates, delegate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("db.GetDelegates: %w", err)
	}
	return delegates, nil
}

// Обновление делегата
func (s *Storage) UpdateDelegate(ctx context.Context, tx pgx.Tx, delegate models.Delegate) error {
	_, err := tx.Exec(ctx,
		"UPDATE delegates SET telegram_id = $1, name = $2, delegate_group = $3, has_voted = $4 WHERE delegate_id = $5",
		delegate.TelegramID, delegate.Name, delegate.Group, delegate.HasVoted, delegate.DelegateID)
	if err != nil {
		return fmt.Errorf("db.UpdateDelegate: %w", err)
	}

	return nil
}

// Удаление делегата
func (s *Storage) DeleteDelegate(ctx context.Context, tx pgx.Tx, delegateID int) error {
	_, err := tx.Exec(ctx, "DELETE FROM delegates WHERE delegate_id = $1", delegateID)
	if err != nil {
		return fmt.Errorf("db.DeleteDelegate: %w", err)
	}

	return nil
}
