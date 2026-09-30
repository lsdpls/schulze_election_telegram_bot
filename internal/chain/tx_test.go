package chain

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	mocks "github.com/lsdpls/schulze_election_telegram_bot/internal/chain/mocks"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"

	"github.com/golang/mock/gomock"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// commitTx — pgx.Tx, у которого Commit может вернуть заданную ошибку (например, 40001 при фиксации)
type commitTx struct {
	pgx.Tx
	commitErr  error
	committed  bool
	rolledBack bool
}

func (c *commitTx) Commit(context.Context) error {
	if c.commitErr != nil {
		return c.commitErr
	}
	c.committed = true
	return nil
}
func (c *commitTx) Rollback(context.Context) error { c.rolledBack = true; return nil }

func pgErr(code string) error {
	// так ошибку оборачивает слой db: fmt.Errorf("...: %w", err)
	return fmt.Errorf("UpsertVote: upsert failed: %w", &pgconn.PgError{Code: code, Message: "test"})
}

// Конфликт сериализации в запросе: транзакция повторяется целиком и фиксируется со второй попытки
func TestAddVoteRetriesSerializationFailure(t *testing.T) {
	st := mocks.NewMockstorage(gomock.NewController(t))
	var txs []*commitTx
	st.EXPECT().BeginTx(gomock.Any(), pgx.TxOptions{IsoLevel: pgx.Serializable}).DoAndReturn(func(context.Context, pgx.TxOptions) (pgx.Tx, error) {
		tx := &commitTx{}
		txs = append(txs, tx)
		return tx, nil
	}).Times(2)
	st.EXPECT().GetDelegateByTelegramID(gomock.Any(), gomock.Any(), int64(42)).Return(&models.Delegate{DelegateID: 123456}, nil).Times(2)
	gomock.InOrder(
		st.EXPECT().UpsertVote(gomock.Any(), gomock.Any(), gomock.Any()).Return(pgErr(pgSerializationFailure)),
		st.EXPECT().UpsertVote(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
	)
	st.EXPECT().UpdateDelegate(gomock.Any(), gomock.Any(), models.Delegate{DelegateID: 123456, HasVoted: true}).Return(nil)

	if err := NewVoteChain(st).AddVote(context.Background(), 42, []int{1, 2, 3}); err != nil {
		t.Fatalf("AddVote: %v", err)
	}
	if len(txs) != 2 || txs[0].committed || !txs[0].rolledBack || !txs[1].committed {
		t.Fatalf("попытки: первая committed=%v rolledBack=%v, вторая committed=%v (всего %d)", txs[0].committed, txs[0].rolledBack, txs[len(txs)-1].committed, len(txs))
	}
}

// 40001 при COMMIT и взаимоблокировка (40P01) тоже повторяются
func TestInTxRetriesOnCommitConflictAndDeadlock(t *testing.T) {
	for _, code := range []string{pgSerializationFailure, pgDeadlockDetected} {
		t.Run(code, func(t *testing.T) {
			st := mocks.NewMockstorage(gomock.NewController(t))
			first := &commitTx{commitErr: &pgconn.PgError{Code: code}}
			second := &commitTx{}
			gomock.InOrder(
				st.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(first, nil),
				st.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(second, nil),
			)
			calls := 0
			err := NewVoteChain(st).inTx(context.Background(), func(pgx.Tx) error { calls++; return nil })
			if err != nil || calls != 2 || !second.committed {
				t.Fatalf("err=%v calls=%d committed=%v", err, calls, second.committed)
			}
		})
	}
}

// Прочие ошибки (в том числе бизнес-ошибки и нарушение уникальности 23505) не повторяются
func TestInTxDoesNotRetryOtherErrors(t *testing.T) {
	for _, e := range []error{errors.New("delegate not found"), pgErr("23505"), fmt.Errorf("x: %w", ErrAlreadyVerified)} {
		st := mocks.NewMockstorage(gomock.NewController(t))
		st.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(&commitTx{}, nil).Times(1)
		calls := 0
		err := NewVoteChain(st).inTx(context.Background(), func(pgx.Tx) error { calls++; return e })
		if !errors.Is(err, e) || calls != 1 {
			t.Fatalf("%v: err=%v calls=%d", e, err, calls)
		}
	}
}

// После maxTxAttempts конфликтов подряд возвращается последняя ошибка, по ней по-прежнему виден код 40001
func TestInTxGivesUpAfterMaxAttempts(t *testing.T) {
	st := mocks.NewMockstorage(gomock.NewController(t))
	st.EXPECT().BeginTx(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, pgx.TxOptions) (pgx.Tx, error) {
		return &commitTx{}, nil
	}).Times(maxTxAttempts)
	var calls int32
	err := NewVoteChain(st).inTx(context.Background(), func(pgx.Tx) error {
		atomic.AddInt32(&calls, 1)
		return pgErr(pgSerializationFailure)
	})
	if calls != maxTxAttempts || !isRetryable(err) {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

// Отмена контекста во время паузы перед повтором прерывает повторы сразу
func TestInTxStopsOnContextCancel(t *testing.T) {
	st := mocks.NewMockstorage(gomock.NewController(t))
	st.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(&commitTx{}, nil).Times(1)
	ctx, cancel := context.WithCancel(context.Background())
	start := time.Now()
	err := NewVoteChain(st).inTx(ctx, func(pgx.Tx) error {
		cancel()
		return pgErr(pgSerializationFailure)
	})
	if err == nil || !isRetryable(err) || time.Since(start) > time.Second {
		t.Fatalf("err=%v elapsed=%v", err, time.Since(start))
	}
}

// Повторный голос перезаписывается тем же UPSERT, has_voted второй раз не пишется
func TestAddVoteRevoteDoesNotRewriteDelegate(t *testing.T) {
	st := mocks.NewMockstorage(gomock.NewController(t))
	tx := &commitTx{}
	st.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil)
	st.EXPECT().GetDelegateByTelegramID(gomock.Any(), tx, int64(42)).Return(&models.Delegate{DelegateID: 123456, HasVoted: true}, nil)
	st.EXPECT().UpsertVote(gomock.Any(), tx, gomock.Any()).DoAndReturn(func(_ context.Context, _ pgx.Tx, v models.Vote) error {
		if v.DelegateID != 123456 || len(v.CandidateRankings) != 2 {
			t.Errorf("vote=%+v", v)
		}
		return nil
	})
	if err := NewVoteChain(st).AddVote(context.Background(), 42, []int{2, 1}); err != nil || !tx.committed {
		t.Fatalf("err=%v committed=%v", err, tx.committed)
	}
}

// Проголосовавшего делегата удалить нельзя (флаг has_voted или строка в votes); не голосовавший удаляется
func TestDeleteDelegateRefusesVoted(t *testing.T) {
	cases := []struct {
		name       string
		hasVoted   bool
		vote       *models.Vote
		wantDelete bool
	}{
		{"не голосовал", false, nil, true},
		{"has_voted", true, nil, false},
		{"есть бюллетень", false, &models.Vote{ID: 1, DelegateID: 123456}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := mocks.NewMockstorage(gomock.NewController(t))
			tx := &commitTx{}
			st.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil)
			st.EXPECT().GetDelegateByDelegateID(gomock.Any(), tx, 123456).Return(&models.Delegate{DelegateID: 123456, HasVoted: c.hasVoted}, nil)
			st.EXPECT().GetVoteByDelegateID(gomock.Any(), tx, 123456).Return(c.vote, nil)
			if c.wantDelete {
				st.EXPECT().DeleteDelegate(gomock.Any(), tx, 123456).Return(nil)
			}
			err := NewVoteChain(st).DeleteDelegate(context.Background(), 123456)
			if c.wantDelete && (err != nil || !tx.committed) {
				t.Fatalf("err=%v committed=%v", err, tx.committed)
			}
			if !c.wantDelete && (!errors.Is(err, ErrDelegateHasVoted) || tx.committed) {
				t.Fatalf("err=%v committed=%v, ожидался ErrDelegateHasVoted без записи", err, tx.committed)
			}
		})
	}
}
