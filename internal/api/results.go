package api

import (
	"context"
	"encoding/json"
	"net/http"

	log "github.com/sirupsen/logrus"
)

type ResultResponse struct {
	Course            string `json:"course"`
	WinnerCandidateID []int  `json:"winner_candidate_id"`
	Preferences       string `json:"preferences"`
	StrongestPaths    string `json:"strongest_paths"`
	Stage             string `json:"stage"`
}

func (h *Handler) GetResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := context.Background()

	results, err := h.voteChain.GetAllResults(ctx)
	if err != nil {
		log.Errorf("Failed to get results: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := make([]ResultResponse, 0, len(results))
	for _, result := range results {
		// Преобразуем map в JSON строку
		preferencesJSON, _ := json.Marshal(result.Preferences)
		strongestPathsJSON, _ := json.Marshal(result.StrongestPaths)

		response = append(response, ResultResponse{
			Course:            result.Course,
			WinnerCandidateID: result.WinnerCandidateID,
			Preferences:       string(preferencesJSON),
			StrongestPaths:    string(strongestPathsJSON),
			Stage:             result.Stage,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Errorf("Failed to encode response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}
