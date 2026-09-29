package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"

	log "github.com/sirupsen/logrus"
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
	GetAllEligibleCandidates(ctx context.Context) ([]models.Candidate, error)
	GetAllResults(ctx context.Context) ([]models.Result, error)
}

// writeJSON отдаёт ответ без кэширования: бюллетени и результаты меняются,
// а в кэше браузера/прокси им делать нечего
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Errorf("Failed to encode response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
