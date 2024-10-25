package chain

import (
	"context"
	"database/sql"
	"fmt"
	"vote_system/internal/models"

	"github.com/jackc/pgx/v5"
)

func (vc *VoteChain) AddDelegate(ctx context.Context, delegate models.Delegate) error {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("chain.AddDelegate: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	delegateDB, err := vc.storage.GetDelegateByDelegateID(ctx, tx, delegate.DelegateID)
	if err != nil {
		return fmt.Errorf("chain.AddDelegate: %w", err)
	}
	if delegateDB != nil {
		return fmt.Errorf("chain.AddDelegate: delegate already exists")
	}

	if err := vc.storage.AddDelegate(ctx, tx, delegate); err != nil {
		return fmt.Errorf("chain.AddDelegate: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("chain.AddDelegate: can't commit transaction: %w", err)
	}
	return nil
}

func (vc *VoteChain) GetDelegateByDelegateID(ctx context.Context, delegateID int) (*models.Delegate, error) {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("chain.GetDelegateByEmail: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	delegate, err := vc.storage.GetDelegateByDelegateID(ctx, tx, delegateID)
	if err != nil {
		return nil, fmt.Errorf("chain.GetDelegateByEmail: %w", err)
	}

	return delegate, nil
}

func (vc *VoteChain) GetAllDelegates(ctx context.Context) ([]models.Delegate, error) {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("chain.GetDelegates: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	delegates, err := vc.storage.GetAllDelegates(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("chain.GetDelegates: %w", err)
	}

	return delegates, nil
}

func (vc *VoteChain) VerificateDelegate(ctx context.Context, delegateID int, telegramId sql.NullInt64) error {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("chain.UpdateDelegate: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	delegate, err := vc.storage.GetDelegateByDelegateID(ctx, tx, delegateID)
	if err != nil {
		return fmt.Errorf("chain.UpdateDelegate: %w", err)
	}
	if delegate == nil {
		return fmt.Errorf("chain.UpdateDelegate: delegate not found")
	}

	delegate.TelegramID = telegramId
	err = vc.storage.UpdateDelegate(ctx, tx, *delegate)
	if err != nil {
		return fmt.Errorf("chain.UpdateDelegate: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("chain.UpdateDelegate: can't commit transaction: %w", err)
	}
	return nil
}

func (vc *VoteChain) DeleteDelegate(ctx context.Context, delegateID int) error {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("chain.DeleteDelegate: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	delegate, err := vc.storage.GetDelegateByDelegateID(ctx, tx, delegateID)
	if err != nil {
		return fmt.Errorf("chain.DeleteDelegate: %w", err)
	}
	if delegate == nil {
		return fmt.Errorf("chain.DeleteDelegate: delegate not found")
	}

	if err := vc.storage.DeleteDelegate(ctx, tx, delegateID); err != nil {
		return fmt.Errorf("chain.DeleteDelegate: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("chain.DeleteDelegate: can't commit transaction: %w", err)
	}
	return nil
}
func (vc *VoteChain) CheckExistDelegateByDelegateID(ctx context.Context, delegateID int) (bool, error) {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return false, fmt.Errorf("chain.CheckExistDelegateByEmail: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	delegate, err := vc.storage.GetDelegateByDelegateID(ctx, tx, delegateID)
	if err != nil {
		return false, fmt.Errorf("chain.CheckExistDelegateByEmail: %w", err)
	}
	if delegate == nil {
		return false, nil
	}
	return true, nil
}

func (vc *VoteChain) CheckExistDelegateByTelegramID(ctx context.Context, telegramID int64) (bool, error) {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return false, fmt.Errorf("chain.CheckExistDelegateByTelegramID: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	delegate, err := vc.storage.GetDelegateByTelegramID(ctx, tx, telegramID)
	if err != nil {
		return false, fmt.Errorf("chain.CheckExistDelegateByTelegramID: %w", err)
	}
	if delegate == nil {
		return false, nil
	}
	return true, nil
}

func (vc *VoteChain) CheckFerification(ctx context.Context, delegateID int) (bool, error) {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return false, fmt.Errorf("chain.CheckFerification: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	delegate, err := vc.storage.GetDelegateByDelegateID(ctx, tx, delegateID)
	if err != nil {
		return false, fmt.Errorf("chain.CheckFerification: %w", err)
	}
	if delegate.TelegramID.Valid {
		return true, nil
	}

	return false, nil
}
