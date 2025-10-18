package api

import (
	"context"
	"encoding/json"
	"net/http"

	log "github.com/sirupsen/logrus"
)

type CandidateResponse struct {
	CandidateID int    `json:"candidate_id"`
	Name        string `json:"name"`
	Course      string `json:"course"`
}

func (h *Handler) GetCandidates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := context.Background()

	candidates, err := h.voteChain.GetAllCandidates(ctx)
	if err != nil {
		log.Errorf("Failed to get candidates: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := make([]CandidateResponse, 0, len(candidates))
	for _, candidate := range candidates {
		response = append(response, CandidateResponse{
			CandidateID: candidate.CandidateID,
			Name:        candidate.Name,
			Course:      candidate.Course,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Errorf("Failed to encode response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}
