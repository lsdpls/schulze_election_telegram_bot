package chain

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	mocks "github.com/lsdpls/schulze_election_telegram_bot/internal/chain/mocks"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"

	"github.com/golang/mock/gomock"
	"github.com/jackc/pgx/v5"
)

// fakeTx — pgx.Tx, у которого нужны только Commit/Rollback
type fakeTx struct {
	pgx.Tx
	committed bool
}

func (f *fakeTx) Commit(context.Context) error   { f.committed = true; return nil }
func (f *fakeTx) Rollback(context.Context) error { return nil }

func nullInt(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }

// Привязка аккаунта: свободный делегат и тот же аккаунт — можно, чужой аккаунт — ErrAlreadyVerified без записи
func TestVerificateDelegateConditions(t *testing.T) {
	cases := []struct {
		name       string
		stored     sql.NullInt64
		incoming   sql.NullInt64
		wantErr    error
		wantUpdate bool
	}{
		{"свободный делегат", sql.NullInt64{}, nullInt(100), nil, true},
		{"тот же аккаунт", nullInt(100), nullInt(100), nil, true},
		{"чужой аккаунт", nullInt(100), nullInt(200), ErrAlreadyVerified, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := mocks.NewMockstorage(gomock.NewController(t))
			tx := &fakeTx{}
			st.EXPECT().BeginTx(gomock.Any(), pgx.TxOptions{IsoLevel: pgx.Serializable}).Return(tx, nil)
			st.EXPECT().GetDelegateByDelegateID(gomock.Any(), tx, 123456).Return(&models.Delegate{DelegateID: 123456, TelegramID: c.stored, Name: "n", Group: "g"}, nil)
			if c.wantUpdate {
				st.EXPECT().UpdateDelegate(gomock.Any(), tx, models.Delegate{DelegateID: 123456, TelegramID: c.incoming, Name: "n", Group: "g"}).Return(nil)
			}
			err := NewVoteChain(st).VerificateDelegate(context.Background(), 123456, c.incoming)
			if c.wantErr == nil && err != nil {
				t.Fatalf("err=%v, want nil", err)
			}
			if c.wantErr != nil && !errors.Is(err, c.wantErr) {
				t.Fatalf("err=%v, want %v", err, c.wantErr)
			}
			if tx.committed != c.wantUpdate {
				t.Fatalf("committed=%v, want %v", tx.committed, c.wantUpdate)
			}
		})
	}
}

// Делегата нет (storage вернул nil, nil): VerificateDelegate — ошибка, CheckFerification — false без паники
func TestNotFoundDelegate(t *testing.T) {
	st := mocks.NewMockstorage(gomock.NewController(t))
	tx := &fakeTx{}
	st.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil).AnyTimes()
	st.EXPECT().GetDelegateByDelegateID(gomock.Any(), tx, 999999).Return(nil, nil).AnyTimes()
	vc := NewVoteChain(st)
	if err := vc.VerificateDelegate(context.Background(), 999999, nullInt(1)); err == nil {
		t.Fatal("VerificateDelegate(not found): want error")
	}
	ok, err := vc.CheckFerification(context.Background(), 999999)
	if ok || err != nil {
		t.Fatalf("CheckFerification(not found) = %v, %v; want false, nil", ok, err)
	}
}
