package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/utils"

	log "github.com/sirupsen/logrus"
)

type VoteResponse struct {
	VoteToken         string `json:"vote_token"`
	CandidateRankings []int  `json:"candidate_rankings"`
	CreatedAt         string `json:"created_at"`
}

func (h *Handler) GetVotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := context.Background()

	votes, err := h.voteChain.GetAllVotes(ctx)
	if err != nil {
		log.Errorf("Failed to get votes: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	delegates, err := h.voteChain.GetAllDelegates(ctx)
	if err != nil {
		log.Errorf("Failed to get delegates: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	delegateMap := make(map[int]int64)
	for _, delegate := range delegates {
		if delegate.TelegramID.Valid {
			delegateMap[delegate.DelegateID] = delegate.TelegramID.Int64
		}
	}

	response := make([]VoteResponse, 0, len(votes))
	for _, vote := range votes {
		telegramID, ok := delegateMap[vote.DelegateID]
		if !ok {
			log.Warnf("Delegate %d has no telegram ID", vote.DelegateID)
			continue
		}

		voteToken := utils.GenerateVoteToken(telegramID)
		response = append(response, VoteResponse{
			VoteToken:         voteToken,
			CandidateRankings: vote.CandidateRankings,
			CreatedAt:         vote.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Errorf("Failed to encode response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}
