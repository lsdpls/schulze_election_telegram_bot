package chain

import (
	"context"
	"fmt"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"

	"github.com/jackc/pgx/v5"
)

func (vc *VoteChain) AddResult(ctx context.Context, result models.Result) error {
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		// Проверяем, существует ли уже результат для данного курса
		resultDB, err := vc.storage.GetResultByCourse(ctx, tx, result.Course)
		if err != nil {
			return err
		}
		if resultDB != nil {
			return vc.storage.UpdateResult(ctx, tx, result)
		}
		// Добавляем результат в базу данных
		return vc.storage.AddResult(ctx, tx, result)
	})
	if err != nil {
		return fmt.Errorf("chain.AddResult: %w", err)
	}
	return nil
}

func (vc *VoteChain) GetResultByCourse(ctx context.Context, course string) (*models.Result, error) {
	var result *models.Result
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		result, err = vc.storage.GetResultByCourse(ctx, tx, course)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("chain.GetResultByCourse: %w", err)
	}
	return result, nil
}

func (vc *VoteChain) GetAllResults(ctx context.Context) ([]models.Result, error) {
	var results []models.Result
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		results, err = vc.storage.GetAllResults(ctx, tx)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("chain.GetAllResults: %w", err)
	}
	return results, nil
}
