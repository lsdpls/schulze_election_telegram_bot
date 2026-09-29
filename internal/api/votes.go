package api

import (
	"net/http"
	"slices"
	"strings"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/utils"

	log "github.com/sirupsen/logrus"
)

// VoteResponse — публичный вид бюллетеня: только псевдонимный токен и ранжирование.
// Времени голосования и ID делегата здесь нет намеренно: по ним бюллетень
// можно сопоставить с записью «Голос учтен» в лог-чате. Бюллетени видны в реальном
// времени (решение владельца), поэтому момент появления токена наблюдаем.
type VoteResponse struct {
	VoteToken         string `json:"vote_token"`
	CandidateRankings []int  `json:"candidate_rankings"`
}

func (h *Handler) GetVotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

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

		response = append(response, VoteResponse{
			VoteToken:         utils.GenerateVoteToken(telegramID),
			CandidateRankings: vote.CandidateRankings,
		})
	}

	// Порядок по токену (HMAC), а не по id/времени из БД: порядок в JSON
	// не должен выдавать очерёдность голосования
	slices.SortFunc(response, func(a, b VoteResponse) int {
		return strings.Compare(a.VoteToken, b.VoteToken)
	})

	writeJSON(w, response)
}
