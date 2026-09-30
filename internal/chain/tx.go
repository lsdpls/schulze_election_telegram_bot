package chain

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Сколько раз выполнять транзакцию, если PostgreSQL отменил её из-за конфликта сериализации или взаимоблокировки.
// Таблица delegates маленькая, блокировки Serializable ложатся на всю страницу, поэтому при всплеске одновременных
// регистраций и голосов одна транзакция может проиграть несколько раз подряд
const maxTxAttempts = 10

// Пауза перед повтором удваивается с txRetryBase до txRetryMax, плюс случайная добавка того же размера,
// чтобы одновременно отменённые транзакции не столкнулись снова. Все паузы вместе — около 2–4 с
const (
	txRetryBase = 10 * time.Millisecond
	txRetryMax  = 400 * time.Millisecond
)

// Коды PostgreSQL, при которых транзакцию безопасно повторить целиком
const (
	pgSerializationFailure = "40001" // serialization_failure: Serializable отменил транзакцию из-за параллельной записи
	pgDeadlockDetected     = "40P01" // deadlock_detected
)

// isRetryable — ошибка означает, что PostgreSQL отменил транзакцию из-за параллельной работы и её можно повторить
func isRetryable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgSerializationFailure || pgErr.Code == pgDeadlockDetected
	}
	return false
}

// inTx выполняет fn в Serializable-транзакции и фиксирует её. Если PostgreSQL отменил транзакцию из-за конфликта
// сериализации (40001) или взаимоблокировки (40P01), транзакция повторяется целиком, всего до maxTxAttempts раз.
// fn должна быть повторяемой: всё, что она возвращает наружу, присваивать заново на каждой попытке.
// Остальные ошибки (в том числе «не найдено», «уже существует») возвращаются сразу, без повтора.
func (vc *VoteChain) inTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	var err error
	for attempt := 1; attempt <= maxTxAttempts; attempt++ {
		err = vc.runTx(ctx, fn)
		if err == nil || !isRetryable(err) || attempt == maxTxAttempts {
			return err
		}
		delay := min(txRetryBase<<(attempt-1), txRetryMax)
		delay += rand.N(delay)
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w (повтор прерван: %v)", err, ctx.Err())
		case <-time.After(delay):
		}
	}
	return err
}

// runTx — одна попытка: begin, fn, commit; при любой ошибке транзакция откатывается
func (vc *VoteChain) runTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("can't commit transaction: %w", err)
	}
	return nil
}
