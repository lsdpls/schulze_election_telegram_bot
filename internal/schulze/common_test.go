package schulze

import (
	"context"
	"fmt"
	"testing"
	"vote_system/internal/models"
	mock "vote_system/internal/schulze/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestSchulze_excludeCourseWinners(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		mockResults      []models.Result
		wantCandidates   []models.Candidate
		wantVotes        []models.Vote
		wantCommonPlaces int
		wantErr          error
	}{
		{
			name: "NoCourseWinners",
			mockResults: []models.Result{
				{Stage: "common", WinnerCandidateID: []int{5, 6, 7, 8, 9, 10}},
			},
			wantCandidates: []models.Candidate{
				{CandidateID: 1, Course: "course1"},
				{CandidateID: 2, Course: "course1"},
				{CandidateID: 3, Course: "course2"},
				{CandidateID: 4, Course: "course2"},
				{CandidateID: 5, Course: "another"},
				{CandidateID: 6, Course: "another"},
				{CandidateID: 7, Course: "another"},
				{CandidateID: 8, Course: "another"},
				{CandidateID: 9, Course: "another"},
				{CandidateID: 10, Course: "another"},
				{CandidateID: 11, Course: "another"},
			},
			wantVotes: []models.Vote{
				{CandidateRankings: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}},
				{CandidateRankings: []int{2, 1, 4, 3, 6, 5, 8, 7, 10, 9, 11}},
			},
			wantCommonPlaces: 10,
			wantErr:          nil,
		},
		{
			name: "OneCourseWinner",
			mockResults: []models.Result{
				{Stage: "absolute", WinnerCandidateID: []int{1}},
				{Stage: "common", WinnerCandidateID: []int{5, 6, 7, 8, 9, 10}},
			},
			wantCandidates: []models.Candidate{
				{CandidateID: 2, Course: "course1"},
				{CandidateID: 3, Course: "course2"},
				{CandidateID: 4, Course: "course2"},
				{CandidateID: 5, Course: "another"},
				{CandidateID: 6, Course: "another"},
				{CandidateID: 7, Course: "another"},
				{CandidateID: 8, Course: "another"},
				{CandidateID: 9, Course: "another"},
				{CandidateID: 10, Course: "another"},
				{CandidateID: 11, Course: "another"},
			},
			wantVotes: []models.Vote{
				{CandidateRankings: []int{2, 3, 4, 5, 6, 7, 8, 9, 10, 11}},
				{CandidateRankings: []int{2, 4, 3, 6, 5, 8, 7, 10, 9, 11}},
			},
			wantCommonPlaces: 9,
			wantErr:          nil,
		},
		{
			name: "TwoCourseWinners",
			mockResults: []models.Result{
				{Stage: "absolute", WinnerCandidateID: []int{1}},
				{Stage: "tie-breaker", WinnerCandidateID: []int{3}},
				{Stage: "common", WinnerCandidateID: []int{5, 6, 7, 8, 9, 10}},
			},
			wantCandidates: []models.Candidate{
				{CandidateID: 2, Course: "course1"},
				{CandidateID: 4, Course: "course2"},
				{CandidateID: 5, Course: "another"},
				{CandidateID: 6, Course: "another"},
				{CandidateID: 7, Course: "another"},
				{CandidateID: 8, Course: "another"},
				{CandidateID: 9, Course: "another"},
				{CandidateID: 10, Course: "another"},
				{CandidateID: 11, Course: "another"},
			},
			wantVotes: []models.Vote{
				{CandidateRankings: []int{2, 4, 5, 6, 7, 8, 9, 10, 11}},
				{CandidateRankings: []int{2, 4, 6, 5, 8, 7, 10, 9, 11}},
			},
			wantCommonPlaces: 8,
			wantErr:          nil,
		},
		{
			name: "InvalidNumberOfWinners",
			mockResults: []models.Result{
				{Course: "course1", Stage: "tie", WinnerCandidateID: []int{1, 2}},
				{Stage: "common", WinnerCandidateID: []int{5, 6, 7, 8, 9, 10}},
			},
			wantCandidates:   nil,
			wantVotes:        nil,
			wantCommonPlaces: 0,
			wantErr:          fmt.Errorf("excludeCourseWinners: invalid number of winners for course course1: 2"),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockChain := mock.NewMockchain(ctrl)

			s := &Schulze{
				voteChain: mockChain,
				candidates: []models.Candidate{
					{CandidateID: 1, Course: "course1"},
					{CandidateID: 2, Course: "course1"},
					{CandidateID: 3, Course: "course2"},
					{CandidateID: 4, Course: "course2"},
					{CandidateID: 5, Course: "another"},
					{CandidateID: 6, Course: "another"},
					{CandidateID: 7, Course: "another"},
					{CandidateID: 8, Course: "another"},
					{CandidateID: 9, Course: "another"},
					{CandidateID: 10, Course: "another"},
					{CandidateID: 11, Course: "another"},
				},
				votes: []models.Vote{
					{CandidateRankings: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}},
					{CandidateRankings: []int{2, 1, 4, 3, 6, 5, 8, 7, 10, 9, 11}},
				},
			}

			mockChain.EXPECT().GetAllResults(context.Background()).Return(tt.mockResults, nil)
			gotCandidates, gotVotes, gotCommonPlaces, gotErr := s.excludeCourseWinners(context.Background(), s.candidates, s.votes)
			if tt.wantErr != nil {
				assert.Error(t, gotErr)
				assert.Equal(t, tt.wantErr.Error(), gotErr.Error())
			} else {
				assert.NoError(t, gotErr)
				assert.Equal(t, tt.wantCandidates, gotCandidates)
				assert.Equal(t, tt.wantVotes, gotVotes)
				assert.Equal(t, tt.wantCommonPlaces, gotCommonPlaces)
			}
		})
	}
}

func TestSchulze_buildStrictOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		candidates       []models.Candidate
		preferences      map[int]map[int]int
		strongestPaths   map[int]map[int]int
		commonPlaces     int
		wantStrictOrder  []models.Candidate
		wantErr          bool
		wantErrMsg       string
		wantCommonPlaces int
	}{
		{
			name:            "NoCandidates",
			candidates:      []models.Candidate{},
			preferences:     map[int]map[int]int{},
			strongestPaths:  map[int]map[int]int{},
			commonPlaces:    3,
			wantStrictOrder: []models.Candidate{},
			wantErr:         false,
			wantErrMsg:      "",
		},
		{
			name: "SimpleOrder",
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
			},
			preferences: map[int]map[int]int{
				1: {2: 2, 3: 3},
				2: {1: 1, 3: 1},
				3: {1: 0, 2: 2},
			},
			strongestPaths: map[int]map[int]int{
				1: {2: 2, 3: 3},
				2: {1: 0, 3: 0},
				3: {1: 0, 2: 2},
			},
			commonPlaces: 2,
			wantStrictOrder: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 3},
			},
			wantErr:    false,
			wantErrMsg: "",
		},
		{
			name: "HardCase",
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
				{CandidateID: 4},
				{CandidateID: 5},
			},
			preferences: map[int]map[int]int{
				1: {2: 20, 3: 26, 4: 30, 5: 22},
				2: {1: 25, 3: 16, 4: 33, 5: 18},
				3: {1: 19, 2: 29, 4: 17, 5: 24},
				4: {1: 15, 2: 12, 3: 28, 5: 14},
				5: {1: 23, 2: 27, 3: 21, 4: 31},
			},
			strongestPaths: map[int]map[int]int{
				1: {2: 28, 3: 28, 4: 30, 5: 24},
				2: {1: 25, 3: 28, 4: 33, 5: 24},
				3: {1: 25, 2: 29, 4: 29, 5: 24},
				4: {1: 25, 2: 28, 3: 28, 5: 24},
				5: {1: 25, 2: 28, 3: 28, 4: 31},
			},
			commonPlaces: 5,
			wantStrictOrder: []models.Candidate{
				{CandidateID: 5},
				{CandidateID: 1},
				{CandidateID: 3},
				{CandidateID: 2},
				{CandidateID: 4},
			},
			wantErr:    false,
			wantErrMsg: "",
		},
		{
			name: "NoStrongestPaths",
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
			},
			preferences: map[int]map[int]int{
				1: {2: 2, 3: 3},
				2: {1: 1, 3: 1},
				3: {1: 0, 2: 2},
			},
			strongestPaths: map[int]map[int]int{},
			commonPlaces:   2,
			wantStrictOrder: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 3},
			},
			wantErr:    true,
			wantErrMsg: "buildStrictOrder: no strongest paths found",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &Schulze{}
			gotStrictOrder, err := s.buildStrictOrder(tt.candidates, tt.preferences, tt.strongestPaths, tt.commonPlaces)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.wantErrMsg, err.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantStrictOrder, gotStrictOrder)
		})
	}
}

func Test_ignoreCandidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		candidates   []models.Candidate
		candidateID  int
		wantFiltered []models.Candidate
	}{
		{
			name: "IgnoreExistingCandidate",
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
			},
			candidateID: 2,
			wantFiltered: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 3},
			},
		},
		{
			name: "IgnoreNonExistingCandidate",
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
			},
			candidateID: 4,
			wantFiltered: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
			},
		},
		{
			name:         "EmptyCandidates",
			candidates:   []models.Candidate{},
			candidateID:  1,
			wantFiltered: []models.Candidate{},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			filtered := ignoreCandidate(tt.candidates, tt.candidateID)
			assert.Equal(t, tt.wantFiltered, filtered)
		})
	}
}
