package chain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"

	"github.com/jackc/pgx/v5"
)

// ErrAlreadyVerified — делегат уже привязан к другому Telegram-аккаунту
var ErrAlreadyVerified = errors.New("delegate already verified by another account")

// ErrDelegateHasVoted — делегат уже проголосовал; удалять его нельзя, иначе вместе с ним удалился бы бюллетень
var ErrDelegateHasVoted = errors.New("delegate has already voted")

func (vc *VoteChain) AddDelegate(ctx context.Context, delegate models.Delegate) error {
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		delegateDB, err := vc.storage.GetDelegateByDelegateID(ctx, tx, delegate.DelegateID)
		if err != nil {
			return err
		}
		if delegateDB != nil {
			return fmt.Errorf("delegate already exists")
		}
		return vc.storage.AddDelegate(ctx, tx, delegate)
	})
	if err != nil {
		return fmt.Errorf("chain.AddDelegate: %w", err)
	}
	return nil
}

func (vc *VoteChain) GetDelegateByDelegateID(ctx context.Context, delegateID int) (*models.Delegate, error) {
	var delegate *models.Delegate
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		delegate, err = vc.storage.GetDelegateByDelegateID(ctx, tx, delegateID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("chain.GetDelegateByEmail: %w", err)
	}
	return delegate, nil
}

func (vc *VoteChain) GetAllDelegates(ctx context.Context) ([]models.Delegate, error) {
	var delegates []models.Delegate
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		delegates, err = vc.storage.GetAllDelegates(ctx, tx)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("chain.GetDelegates: %w", err)
	}
	return delegates, nil
}

func (vc *VoteChain) VerificateDelegate(ctx context.Context, delegateID int, telegramId sql.NullInt64) error {
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		delegate, err := vc.storage.GetDelegateByDelegateID(ctx, tx, delegateID)
		if err != nil {
			return err
		}
		if delegate == nil {
			return fmt.Errorf("delegate not found")
		}
		// Повторная привязка перехватила бы уже зарегистрированного делегата (устаревший код с другого аккаунта)
		if delegate.TelegramID.Valid && delegate.TelegramID.Int64 != telegramId.Int64 {
			return fmt.Errorf("delegate %d: %w", delegateID, ErrAlreadyVerified)
		}
		delegate.TelegramID = telegramId
		return vc.storage.UpdateDelegate(ctx, tx, *delegate)
	})
	if err != nil {
		return fmt.Errorf("chain.UpdateDelegate: %w", err)
	}
	return nil
}

// DeleteDelegate удаляет делегата, только если он не голосовал: проверка и удаление в одной транзакции,
// поэтому голос, поданный одновременно с удалением, не пропадёт (конфликт сериализации → повтор → отказ)
func (vc *VoteChain) DeleteDelegate(ctx context.Context, delegateID int) error {
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		delegate, err := vc.storage.GetDelegateByDelegateID(ctx, tx, delegateID)
		if err != nil {
			return err
		}
		if delegate == nil {
			return fmt.Errorf("delegate not found")
		}
		vote, err := vc.storage.GetVoteByDelegateID(ctx, tx, delegateID)
		if err != nil {
			return err
		}
		if delegate.HasVoted || vote != nil {
			return fmt.Errorf("delegate %d: %w", delegateID, ErrDelegateHasVoted)
		}
		return vc.storage.DeleteDelegate(ctx, tx, delegateID)
	})
	if err != nil {
		return fmt.Errorf("chain.DeleteDelegate: %w", err)
	}
	return nil
}

func (vc *VoteChain) CheckExistDelegateByDelegateID(ctx context.Context, delegateID int) (bool, error) {
	var exists bool
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		delegate, err := vc.storage.GetDelegateByDelegateID(ctx, tx, delegateID)
		exists = delegate != nil
		return err
	})
	if err != nil {
		return false, fmt.Errorf("chain.CheckExistDelegateByEmail: %w", err)
	}
	return exists, nil
}

func (vc *VoteChain) CheckExistDelegateByTelegramID(ctx context.Context, telegramID int64) (bool, error) {
	var exists bool
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		delegate, err := vc.storage.GetDelegateByTelegramID(ctx, tx, telegramID)
		exists = delegate != nil
		return err
	})
	if err != nil {
		return false, fmt.Errorf("chain.CheckExistDelegateByTelegramID: %w", err)
	}
	return exists, nil
}

func (vc *VoteChain) CheckFerification(ctx context.Context, delegateID int) (bool, error) {
	var verified bool
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		delegate, err := vc.storage.GetDelegateByDelegateID(ctx, tx, delegateID)
		// нет делегата — нет и верификации
		verified = delegate != nil && delegate.TelegramID.Valid
		return err
	})
	if err != nil {
		return false, fmt.Errorf("chain.CheckFerification: %w", err)
	}
	return verified, nil
}
