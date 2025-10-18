package chain

import (
	"context"
	"fmt"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"

	"github.com/jackc/pgx/v5"
)

func (vc *VoteChain) AddResult(ctx context.Context, result models.Result) error {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("chain.AddResult: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Проверяем, существует ли уже результат для данного курса
	resultDB, err := vc.storage.GetResultByCourse(ctx, tx, result.Course)
	if err != nil {
		return fmt.Errorf("chain.AddResult: %w", err)
	}
	if resultDB != nil {
		if err := vc.storage.UpdateResult(ctx, tx, result); err != nil {
			return fmt.Errorf("chain.AddResult: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("chain.AddResult: can't commit transaction: %w", err)
		}
		return nil
	}

	// Добавляем результат в базу данных
	if err := vc.storage.AddResult(ctx, tx, result); err != nil {
		return fmt.Errorf("chain.AddResult: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("chain.AddResult: can't commit transaction: %w", err)
	}
	return nil
}

func (vc *VoteChain) GetResultByCourse(ctx context.Context, course string) (*models.Result, error) {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("chain.GetResultByCourse: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	result, err := vc.storage.GetResultByCourse(ctx, tx, course)
	if err != nil {
		return nil, fmt.Errorf("chain.GetResultByCourse: %w", err)
	}

	return result, nil
}

func (vc *VoteChain) GetAllResults(ctx context.Context) ([]models.Result, error) {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("chain.GetAllResults: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	results, err := vc.storage.GetAllResults(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("chain.GetAllResults: %w", err)
	}

	return results, nil
}
