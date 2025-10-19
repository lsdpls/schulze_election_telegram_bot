package api

import (
	"context"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"
)

type Handler struct {
	voteChain voteChain
}

func NewHandler(voteChain voteChain) *Handler {
	return &Handler{
		voteChain: voteChain,
	}
}

type voteChain interface {
	GetAllVotes(ctx context.Context) ([]models.Vote, error)
	GetAllDelegates(ctx context.Context) ([]models.Delegate, error)
	GetAllCandidates(ctx context.Context) ([]models.Candidate, error)
	GetAllResults(ctx context.Context) ([]models.Result, error)
}
