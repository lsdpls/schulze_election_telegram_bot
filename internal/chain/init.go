package chain

import (
	"context"
	"vote_system/internal/models"

	"github.com/jackc/pgx/v5"
)

// Структура для управления пользователями через базу данных
type VoteChain struct {
	storage storage
}

// Конструктор для создания нового VoteChain
func NewVoteChain(storage storage) *VoteChain {
	return &VoteChain{
		storage: storage,
	}
}

type storage interface {
	AddDelegate(ctx context.Context, tx pgx.Tx, delegate models.Delegate) error
	GetDelegateByDelegateID(ctx context.Context, tx pgx.Tx, delegateID int) (*models.Delegate, error)
	GetDelegateByTelegramID(ctx context.Context, tx pgx.Tx, telegramID int64) (*models.Delegate, error)
	GetAllDelegates(ctx context.Context, tx pgx.Tx) ([]models.Delegate, error)
	UpdateDelegate(ctx context.Context, tx pgx.Tx, delegate models.Delegate) error
	DeleteDelegate(ctx context.Context, tx pgx.Tx, delegateID int) error

	AddCandidate(ctx context.Context, tx pgx.Tx, candidate models.Candidate) error
	GetCandidateByCandidateID(ctx context.Context, tx pgx.Tx, candidateID int) (*models.Candidate, error)
	GetAllCandidates(ctx context.Context, tx pgx.Tx) ([]models.Candidate, error)
	GetAllEligibleCandidates(ctx context.Context, tx pgx.Tx) ([]models.Candidate, error)
	UpdateCandidate(ctx context.Context, tx pgx.Tx, candidate models.Candidate) error
	DeleteCandidate(ctx context.Context, tx pgx.Tx, candidateID int) error

	AddVote(ctx context.Context, tx pgx.Tx, vote models.Vote) error
	GetVoteByDelegateID(ctx context.Context, tx pgx.Tx, delegateID int) (*models.Vote, error)
	GetAllVotes(ctx context.Context, tx pgx.Tx) ([]models.Vote, error)
	UpdateVote(ctx context.Context, tx pgx.Tx, vote models.Vote) error
	DeleteVote(ctx context.Context, tx pgx.Tx, voteID int) error

	AddResult(ctx context.Context, tx pgx.Tx, result models.Result) error
	GetResultByCourse(ctx context.Context, tx pgx.Tx, course string) (*models.Result, error)
	GetAllResults(ctx context.Context, tx pgx.Tx) ([]models.Result, error)
	UpdateResult(ctx context.Context, tx pgx.Tx, result models.Result) error
	DeleteResult(ctx context.Context, tx pgx.Tx, resultID int) error

	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}
