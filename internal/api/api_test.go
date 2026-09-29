package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/config"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/utils"
)

// fakeChain — ручная заглушка voteChain без моков
type fakeChain struct {
	votes      []models.Vote
	delegates  []models.Delegate
	candidates []models.Candidate
	results    []models.Result
	err        error
}

func (f *fakeChain) GetAllVotes(context.Context) ([]models.Vote, error) {
	return f.votes, f.err
}
func (f *fakeChain) GetAllDelegates(context.Context) ([]models.Delegate, error) {
	return f.delegates, f.err
}
func (f *fakeChain) GetAllCandidates(context.Context) ([]models.Candidate, error) {
	return f.candidates, f.err
}
func (f *fakeChain) GetAllEligibleCandidates(context.Context) ([]models.Candidate, error) {
	var out []models.Candidate
	for _, c := range f.candidates {
		if c.IsEligible {
			out = append(out, c)
		}
	}
	return out, f.err
}
func (f *fakeChain) GetAllResults(context.Context) ([]models.Result, error) {
	return f.results, f.err
}

func tgID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: true} }

func init() {
	// GenerateVoteToken читает секрет из пакетного глобала config
	config.VoteTokenSecret = "test-secret-test-secret-test-secret-32"
}

func newVotesFixture() *fakeChain {
	base := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	return &fakeChain{
		// три бюллетеня в порядке вставки (id/created_at растут), четвёртый — от делегата без telegram_id
		votes: []models.Vote{
			{ID: 1, DelegateID: 100001, CandidateRankings: []int{1, 2, 3}, CreatedAt: base},
			{ID: 2, DelegateID: 100002, CandidateRankings: []int{2, 1, 3}, CreatedAt: base.Add(time.Minute)},
			{ID: 3, DelegateID: 100003, CandidateRankings: []int{3, 2, 1}, CreatedAt: base.Add(2 * time.Minute)},
			{ID: 4, DelegateID: 100004, CandidateRankings: []int{1, 3, 2}, CreatedAt: base.Add(3 * time.Minute)},
		},
		delegates: []models.Delegate{
			{DelegateID: 100001, TelegramID: tgID(1111)},
			{DelegateID: 100002, TelegramID: tgID(2222)},
			{DelegateID: 100003, TelegramID: tgID(3333)},
			{DelegateID: 100004}, // не зарегистрирован в Telegram
		},
	}
}

func doGet(t *testing.T, h http.HandlerFunc, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d, body %q", path, rec.Code, rec.Body.String())
	}
	return rec
}

func TestGetVotes_NoDelegateDataInJSON(t *testing.T) {
	rec := doGet(t, NewHandler(newVotesFixture()).GetVotes, "/votes")

	var raw []map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("empty response")
	}
	for i, vote := range raw {
		for _, key := range []string{"created_at", "delegate_id", "telegram_id", "id"} {
			if _, ok := vote[key]; ok {
				t.Errorf("vote %d: field %q must not be exposed", i, key)
			}
		}
		if len(vote) != 2 {
			t.Errorf("vote %d: expected exactly vote_token and candidate_rankings, got %d fields", i, len(vote))
		}
	}
	body := rec.Body.String()
	for _, needle := range []string{"2026-09-29", "100001", "1111"} {
		if strings.Contains(body, needle) {
			t.Errorf("body leaks %q: %s", needle, body)
		}
	}
}

func TestGetVotes_SortedByTokenAndSkipsUnregistered(t *testing.T) {
	rec := doGet(t, NewHandler(newVotesFixture()).GetVotes, "/votes")

	var got []VoteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 votes (delegate without telegram id skipped), got %d", len(got))
	}

	tokens := make([]string, 0, len(got))
	for _, v := range got {
		tokens = append(tokens, v.VoteToken)
	}
	if !slices.IsSorted(tokens) {
		t.Errorf("votes not sorted by token: %v", tokens)
	}

	// каждый токен соответствует зарегистрированному делегату и его бюллетеню
	want := map[string][]int{
		utils.GenerateVoteToken(1111): {1, 2, 3},
		utils.GenerateVoteToken(2222): {2, 1, 3},
		utils.GenerateVoteToken(3333): {3, 2, 1},
	}
	for _, v := range got {
		rank, ok := want[v.VoteToken]
		if !ok {
			t.Errorf("unexpected token %q", v.VoteToken)
			continue
		}
		if !slices.Equal(rank, v.CandidateRankings) {
			t.Errorf("token %q: rankings %v, want %v", v.VoteToken, v.CandidateRankings, rank)
		}
	}
}

func TestGetVotes_OrderIndependentOfInsertion(t *testing.T) {
	// Разный порядок из БД → одинаковый JSON
	a := newVotesFixture()
	b := newVotesFixture()
	slices.Reverse(b.votes)

	bodyA := doGet(t, NewHandler(a).GetVotes, "/votes").Body.String()
	bodyB := doGet(t, NewHandler(b).GetVotes, "/votes").Body.String()
	if bodyA != bodyB {
		t.Errorf("response depends on DB order:\n%s\n%s", bodyA, bodyB)
	}
}

func TestHandlers_HeadersAndMethods(t *testing.T) {
	h := NewHandler(&fakeChain{})
	handlers := map[string]http.HandlerFunc{
		"/votes":      h.GetVotes,
		"/candidates": h.GetCandidates,
		"/result":     h.GetResults,
	}
	for path, fn := range handlers {
		rec := doGet(t, fn, path)
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: Cache-Control = %q, want no-store", path, got)
		}
		if got := rec.Header().Get("Content-Type"); got != "application/json" {
			t.Errorf("%s: Content-Type = %q", path, got)
		}
		if strings.TrimSpace(rec.Body.String()) != "[]" {
			t.Errorf("%s: empty data must encode as [], got %q", path, rec.Body.String())
		}

		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
			rec := httptest.NewRecorder()
			fn(rec, httptest.NewRequest(method, path, nil))
			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s: status %d, want 405", method, path, rec.Code)
			}
		}
	}
}

func TestHandlers_ChainError(t *testing.T) {
	h := NewHandler(&fakeChain{err: context.DeadlineExceeded})
	for path, fn := range map[string]http.HandlerFunc{"/votes": h.GetVotes, "/candidates": h.GetCandidates, "/result": h.GetResults} {
		rec := httptest.NewRecorder()
		fn(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s: status %d, want 500", path, rec.Code)
		}
	}
}

func TestGetCandidates_PublicFieldsOnly(t *testing.T) {
	h := NewHandler(&fakeChain{candidates: []models.Candidate{
		{CandidateID: 123456, Name: "Иванов И.И.", Course: "2 бакалавриат", Description: "SECRET-DESC", IsEligible: true},
	}})
	rec := doGet(t, h.GetCandidates, "/candidates")

	var raw []map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(raw) != 1 || len(raw[0]) != 3 {
		t.Fatalf("expected one candidate with 3 fields, got %v", raw)
	}
	for _, key := range []string{"candidate_id", "name", "course"} {
		if _, ok := raw[0][key]; !ok {
			t.Errorf("missing %q", key)
		}
	}
	if strings.Contains(rec.Body.String(), "SECRET-DESC") {
		t.Error("description must not be exposed")
	}
}

func TestGetResults_Shape(t *testing.T) {
	h := NewHandler(&fakeChain{results: []models.Result{{
		ID:                7,
		Course:            "2 бакалавриат",
		WinnerCandidateID: []int{123456},
		Preferences:       map[int]map[int]int{123456: {654321: 5}, 654321: {123456: 2}},
		StrongestPaths:    map[int]map[int]int{123456: {654321: 5}, 654321: {123456: 0}},
		Stage:             "absolute",
	}}})
	rec := doGet(t, h.GetResults, "/result")

	var got []ResultResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(got) != 1 || got[0].Course != "2 бакалавриат" || got[0].Stage != "absolute" || !slices.Equal(got[0].WinnerCandidateID, []int{123456}) {
		t.Fatalf("unexpected result: %+v", got)
	}
	var prefs map[string]map[string]int
	if err := json.Unmarshal([]byte(got[0].Preferences), &prefs); err != nil || prefs["123456"]["654321"] != 5 {
		t.Errorf("preferences matrix not round-tripped: %q (%v)", got[0].Preferences, err)
	}
	var raw []map[string]json.RawMessage
	_ = json.Unmarshal(rec.Body.Bytes(), &raw)
	if _, ok := raw[0]["id"]; ok {
		t.Error("internal id must not be exposed")
	}
}

// Недопущенные кандидаты (is_eligible=false) на страницу не попадают
func TestGetCandidates_EligibleOnly(t *testing.T) {
	h := NewHandler(&fakeChain{candidates: []models.Candidate{
		{CandidateID: 1, Name: "Допущен", Course: "1 бакалавриат", IsEligible: true},
		{CandidateID: 2, Name: "Забанен", Course: "1 бакалавриат", IsEligible: false},
	}})
	body := doGet(t, h.GetCandidates, "/candidates").Body.String()
	if !strings.Contains(body, "Допущен") || strings.Contains(body, "Забанен") {
		t.Fatalf("ожидались только допущенные: %s", body)
	}
}
