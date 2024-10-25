package schulze

import (
	"reflect"
	"testing"
	"vote_system/internal/models"

	"github.com/stretchr/testify/assert"
)

func TestSchulze_computePairwisePreferences(t *testing.T) {
	t.Parallel()
	s := &Schulze{}
	tests := []struct {
		name       string
		votes      []models.Vote
		candidates []models.Candidate
		want       map[int]map[int]int
	}{
		{
			name: "SimpleCase",
			votes: []models.Vote{
				{CandidateRankings: []int{1, 2, 3}},
				{CandidateRankings: []int{2, 3, 1}},
				{CandidateRankings: []int{3, 1, 2}},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
			},
			want: map[int]map[int]int{
				1: {2: 2, 3: 1},
				2: {1: 1, 3: 2},
				3: {1: 2, 2: 1},
			},
		},
		{
			name: "HardCase", // https://en.wikipedia.org/wiki/Schulze_method
			votes: []models.Vote{
				// 5 ACBED
				{CandidateRankings: []int{1, 3, 2, 5, 4}},
				{CandidateRankings: []int{1, 3, 2, 5, 4}},
				{CandidateRankings: []int{1, 3, 2, 5, 4}},
				{CandidateRankings: []int{1, 3, 2, 5, 4}},
				{CandidateRankings: []int{1, 3, 2, 5, 4}},
				// 5 ADECB
				{CandidateRankings: []int{1, 4, 5, 3, 2}},
				{CandidateRankings: []int{1, 4, 5, 3, 2}},
				{CandidateRankings: []int{1, 4, 5, 3, 2}},
				{CandidateRankings: []int{1, 4, 5, 3, 2}},
				{CandidateRankings: []int{1, 4, 5, 3, 2}},
				// 8 BEDAC
				{CandidateRankings: []int{2, 5, 4, 1, 3}},
				{CandidateRankings: []int{2, 5, 4, 1, 3}},
				{CandidateRankings: []int{2, 5, 4, 1, 3}},
				{CandidateRankings: []int{2, 5, 4, 1, 3}},
				{CandidateRankings: []int{2, 5, 4, 1, 3}},
				{CandidateRankings: []int{2, 5, 4, 1, 3}},
				{CandidateRankings: []int{2, 5, 4, 1, 3}},
				{CandidateRankings: []int{2, 5, 4, 1, 3}},
				// 3 CABED
				{CandidateRankings: []int{3, 1, 2, 5, 4}},
				{CandidateRankings: []int{3, 1, 2, 5, 4}},
				{CandidateRankings: []int{3, 1, 2, 5, 4}},
				// 7 CAEBD
				{CandidateRankings: []int{3, 1, 5, 2, 4}},
				{CandidateRankings: []int{3, 1, 5, 2, 4}},
				{CandidateRankings: []int{3, 1, 5, 2, 4}},
				{CandidateRankings: []int{3, 1, 5, 2, 4}},
				{CandidateRankings: []int{3, 1, 5, 2, 4}},
				{CandidateRankings: []int{3, 1, 5, 2, 4}},
				{CandidateRankings: []int{3, 1, 5, 2, 4}},
				// 2 CBADE
				{CandidateRankings: []int{3, 2, 1, 4, 5}},
				{CandidateRankings: []int{3, 2, 1, 4, 5}},
				// 7 DCEBA
				{CandidateRankings: []int{4, 3, 5, 2, 1}},
				{CandidateRankings: []int{4, 3, 5, 2, 1}},
				{CandidateRankings: []int{4, 3, 5, 2, 1}},
				{CandidateRankings: []int{4, 3, 5, 2, 1}},
				{CandidateRankings: []int{4, 3, 5, 2, 1}},
				{CandidateRankings: []int{4, 3, 5, 2, 1}},
				{CandidateRankings: []int{4, 3, 5, 2, 1}},
				// 8 EBADC
				{CandidateRankings: []int{5, 2, 1, 4, 3}},
				{CandidateRankings: []int{5, 2, 1, 4, 3}},
				{CandidateRankings: []int{5, 2, 1, 4, 3}},
				{CandidateRankings: []int{5, 2, 1, 4, 3}},
				{CandidateRankings: []int{5, 2, 1, 4, 3}},
				{CandidateRankings: []int{5, 2, 1, 4, 3}},
				{CandidateRankings: []int{5, 2, 1, 4, 3}},
				{CandidateRankings: []int{5, 2, 1, 4, 3}},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
				{CandidateID: 4},
				{CandidateID: 5},
			},
			want: map[int]map[int]int{
				1: {2: 20, 3: 26, 4: 30, 5: 22},
				2: {1: 25, 3: 16, 4: 33, 5: 18},
				3: {1: 19, 2: 29, 4: 17, 5: 24},
				4: {1: 15, 2: 12, 3: 28, 5: 14},
				5: {1: 23, 2: 27, 3: 21, 4: 31},
			},
		},
		{
			name: "Example12", // https://arxiv.org/pdf/1804.02973
			votes: []models.Vote{
				// 1 ADBEC
				{CandidateRankings: []int{1, 4, 2, 5, 3}},
				{CandidateRankings: []int{1, 4, 2, 5, 3}},
				{CandidateRankings: []int{1, 4, 2, 5, 3}},
				{CandidateRankings: []int{1, 4, 2, 5, 3}},
				{CandidateRankings: []int{1, 4, 2, 5, 3}},
				{CandidateRankings: []int{1, 4, 2, 5, 3}},
				{CandidateRankings: []int{1, 4, 2, 5, 3}},
				{CandidateRankings: []int{1, 4, 2, 5, 3}},
				{CandidateRankings: []int{1, 4, 2, 5, 3}},
				// 1 BACED
				{CandidateRankings: []int{2, 1, 3, 5, 4}},
				// 6 CBADE
				{CandidateRankings: []int{3, 2, 1, 4, 5}},
				{CandidateRankings: []int{3, 2, 1, 4, 5}},
				{CandidateRankings: []int{3, 2, 1, 4, 5}},
				{CandidateRankings: []int{3, 2, 1, 4, 5}},
				{CandidateRankings: []int{3, 2, 1, 4, 5}},
				{CandidateRankings: []int{3, 2, 1, 4, 5}},
				// 2 CDBEA
				{CandidateRankings: []int{3, 4, 2, 5, 1}},
				{CandidateRankings: []int{3, 4, 2, 5, 1}},
				// 5 CDEAB
				{CandidateRankings: []int{3, 4, 5, 1, 2}},
				{CandidateRankings: []int{3, 4, 5, 1, 2}},
				{CandidateRankings: []int{3, 4, 5, 1, 2}},
				{CandidateRankings: []int{3, 4, 5, 1, 2}},
				{CandidateRankings: []int{3, 4, 5, 1, 2}},
				// 6 DECAB
				{CandidateRankings: []int{4, 5, 3, 1, 2}},
				{CandidateRankings: []int{4, 5, 3, 1, 2}},
				{CandidateRankings: []int{4, 5, 3, 1, 2}},
				{CandidateRankings: []int{4, 5, 3, 1, 2}},
				{CandidateRankings: []int{4, 5, 3, 1, 2}},
				{CandidateRankings: []int{4, 5, 3, 1, 2}},
				// 14 EBACD
				{CandidateRankings: []int{5, 2, 1, 3, 4}},
				{CandidateRankings: []int{5, 2, 1, 3, 4}},
				{CandidateRankings: []int{5, 2, 1, 3, 4}},
				{CandidateRankings: []int{5, 2, 1, 3, 4}},
				{CandidateRankings: []int{5, 2, 1, 3, 4}},
				{CandidateRankings: []int{5, 2, 1, 3, 4}},
				{CandidateRankings: []int{5, 2, 1, 3, 4}},
				{CandidateRankings: []int{5, 2, 1, 3, 4}},
				{CandidateRankings: []int{5, 2, 1, 3, 4}},
				{CandidateRankings: []int{5, 2, 1, 3, 4}},
				{CandidateRankings: []int{5, 2, 1, 3, 4}},
				{CandidateRankings: []int{5, 2, 1, 3, 4}},
				{CandidateRankings: []int{5, 2, 1, 3, 4}},
				{CandidateRankings: []int{5, 2, 1, 3, 4}},
				// 2 EBCAD
				{CandidateRankings: []int{5, 2, 3, 1, 4}},
				{CandidateRankings: []int{5, 2, 3, 1, 4}},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
				{CandidateID: 4},
				{CandidateID: 5},
			},
			want: map[int]map[int]int{
				1: {2: 20, 3: 24, 4: 32, 5: 16},
				2: {1: 25, 3: 26, 4: 23, 5: 18},
				3: {1: 21, 2: 19, 4: 30, 5: 14},
				4: {1: 13, 2: 22, 3: 15, 5: 28},
				5: {1: 29, 2: 27, 3: 31, 4: 17},
			},
		},
		{
			name: "OneCandidate",
			votes: []models.Vote{
				{CandidateRankings: []int{1}},
				{CandidateRankings: []int{1}},
				{CandidateRankings: []int{1}},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
			},
			want: map[int]map[int]int{
				1: {},
			},
		},
		{
			name: "NoCandidates",
			votes: []models.Vote{
				{CandidateRankings: []int{}},
				{CandidateRankings: []int{}},
				{CandidateRankings: []int{}},
			},
			candidates: []models.Candidate{},
			want:       map[int]map[int]int{},
		},
		{
			name: "NoVotes",
			votes: []models.Vote{
				{CandidateRankings: []int{}},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
			},
			want: map[int]map[int]int{
				1: {2: 0, 3: 0},
				2: {1: 0, 3: 0},
				3: {1: 0, 2: 0},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := s.computePairwisePreferences(tt.votes, tt.candidates); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("computePairwisePreferences() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSchulze_computeStrongestPaths(t *testing.T) {
	t.Parallel()
	s := &Schulze{}
	tests := []struct {
		name        string
		preferences map[int]map[int]int
		candidates  []models.Candidate
		want        map[int]map[int]int
	}{
		{
			name: "SimpleCase",
			preferences: map[int]map[int]int{
				1: {2: 2, 3: 1},
				2: {1: 1, 3: 2},
				3: {1: 2, 2: 1},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
			},
			want: map[int]map[int]int{
				1: {2: 2, 3: 2},
				2: {1: 2, 3: 2},
				3: {1: 2, 2: 2},
			},
		},
		{
			name: "TwoCandidates",
			preferences: map[int]map[int]int{
				1: {2: 2},
				2: {1: 1},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
			},
			want: map[int]map[int]int{
				1: {2: 2},
				2: {1: 0},
			},
		},
		{
			name: "OneCandidate",
			preferences: map[int]map[int]int{
				1: {},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
			},
			want: map[int]map[int]int{
				1: {},
			},
		},
		{
			name:        "NoCandidates",
			preferences: map[int]map[int]int{},
			candidates:  []models.Candidate{},
			want:        map[int]map[int]int{},
		},
		{
			name: "MediumCase",
			preferences: map[int]map[int]int{
				1: {2: 2, 3: 1, 4: 1},
				2: {1: 0, 3: 3, 4: 0},
				3: {1: 0, 2: 0, 4: 1},
				4: {1: 0, 2: 0, 3: 0},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
				{CandidateID: 4},
			},
			want: map[int]map[int]int{
				1: {2: 2, 3: 2, 4: 1},
				2: {1: 0, 3: 3, 4: 1},
				3: {1: 0, 2: 0, 4: 1},
				4: {1: 0, 2: 0, 3: 0},
			},
		},
		{
			name: "HardCase", // https://en.wikipedia.org/wiki/Schulze_method
			preferences: map[int]map[int]int{
				1: {2: 20, 3: 26, 4: 30, 5: 22},
				2: {1: 25, 3: 16, 4: 33, 5: 18},
				3: {1: 19, 2: 29, 4: 17, 5: 24},
				4: {1: 15, 2: 12, 3: 28, 5: 14},
				5: {1: 23, 2: 27, 3: 21, 4: 31},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
				{CandidateID: 4},
				{CandidateID: 5},
			},
			want: map[int]map[int]int{
				1: {2: 28, 3: 28, 4: 30, 5: 24},
				2: {1: 25, 3: 28, 4: 33, 5: 24},
				3: {1: 25, 2: 29, 4: 29, 5: 24},
				4: {1: 25, 2: 28, 3: 28, 5: 24},
				5: {1: 25, 2: 28, 3: 28, 4: 31},
			},
		},
		{
			name: "Example4", // https://electowiki.org/wiki/Schulze_method
			preferences: map[int]map[int]int{
				1: {2: 5, 3: 5, 4: 3},
				2: {1: 4, 3: 7, 4: 5},
				3: {1: 4, 2: 2, 4: 5},
				4: {1: 6, 2: 4, 3: 4},
			},
			candidates: []models.Candidate{
				{CandidateID: 4},
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
			},
			want: map[int]map[int]int{
				1: {2: 5, 3: 5, 4: 5},
				2: {1: 5, 3: 7, 4: 5},
				3: {1: 5, 2: 5, 4: 5},
				4: {1: 6, 2: 5, 3: 5},
			},
		},
		{
			name: "Example12", // https://arxiv.org/pdf/1804.02973
			preferences: map[int]map[int]int{
				1: {2: 20, 3: 24, 4: 32, 5: 16},
				2: {1: 25, 3: 26, 4: 23, 5: 18},
				3: {1: 21, 2: 19, 4: 30, 5: 14},
				4: {1: 13, 2: 22, 3: 15, 5: 28},
				5: {1: 29, 2: 27, 3: 31, 4: 17},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
				{CandidateID: 4},
				{CandidateID: 5},
			},
			want: map[int]map[int]int{
				1: {2: 27, 3: 28, 4: 32, 5: 28},
				2: {1: 26, 3: 26, 4: 26, 5: 26},
				3: {1: 28, 2: 27, 4: 30, 5: 28},
				4: {1: 28, 2: 27, 3: 28, 5: 28},
				5: {1: 29, 2: 27, 3: 31, 4: 30},
			},
		},
		// TODO: Add more test cases here
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := s.computeStrongestPaths(tt.preferences, tt.candidates); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("computeStrongestPaths() =\n pass %v !=\n want %v", got, tt.want)
			}
		})
	}
}

func TestSchulze_findPotentialWinners(t *testing.T) {
	t.Parallel()
	s := &Schulze{}
	tests := []struct {
		name           string
		strongestPaths map[int]map[int]int
		candidates     []models.Candidate
		want           []models.Candidate
	}{

		{
			name: "NoWinners",
			strongestPaths: map[int]map[int]int{
				1: {2: 1, 3: 2},
				2: {1: 2, 3: 1},
				3: {1: 1, 2: 2},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
			},
			want: []models.Candidate{},
		},
		{
			name: "2_StrongWinners",
			strongestPaths: map[int]map[int]int{
				1: {2: 2, 3: 2},
				2: {1: 2, 3: 2},
				3: {1: 1, 2: 1},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
			},
			want: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
			},
		},
		{
			name: "2_SimpleWinners",
			strongestPaths: map[int]map[int]int{
				1: {2: 2, 3: 2},
				2: {1: 2, 3: 1},
				3: {1: 1, 2: 1},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
			},
			want: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
			},
		},
		{
			name: "3_SimpleWinners",
			strongestPaths: map[int]map[int]int{
				1: {2: 2, 3: 2},
				2: {1: 2, 3: 2},
				3: {1: 2, 2: 2},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
			},
			want: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
			},
		},
		{
			name: "HardCase", // https://en.wikipedia.org/wiki/Schulze_method
			strongestPaths: map[int]map[int]int{
				1: {2: 28, 3: 28, 4: 30, 5: 24},
				2: {1: 25, 3: 28, 4: 33, 5: 24},
				3: {1: 25, 2: 29, 4: 29, 5: 24},
				4: {1: 25, 2: 28, 3: 28, 5: 24},
				5: {1: 25, 2: 28, 3: 28, 4: 31},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
				{CandidateID: 4},
				{CandidateID: 5},
			},
			want: []models.Candidate{{CandidateID: 5}},
		},
		{
			name: "Example4", // https://electowiki.org/wiki/Schulze
			strongestPaths: map[int]map[int]int{
				1: {2: 5, 3: 5, 4: 5},
				2: {1: 5, 3: 7, 4: 5},
				3: {1: 5, 2: 5, 4: 5},
				4: {1: 6, 2: 5, 3: 5},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
				{CandidateID: 4},
			},
			want: []models.Candidate{
				{CandidateID: 2},
				{CandidateID: 4},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := s.findPotentialWinners(tt.strongestPaths, tt.candidates); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Schulze.findPotentialWinner() =\n pass %v,\n want %v", got, tt.want)
			}
		})
	}
}

func TestSchulze_findAllWeakestEdges(t *testing.T) {
	t.Parallel()
	s := &Schulze{}
	tests := []struct {
		name           string
		preferences    map[int]map[int]int
		strongestPaths map[int]map[int]int
		start          int
		end            int
		candidates     []models.Candidate
		want           [][]int
	}{
		{
			name: "SimpleCase",
			preferences: map[int]map[int]int{
				1: {2: 2, 3: 1},
				2: {1: 1, 3: 2},
				3: {1: 2, 2: 1},
			},
			strongestPaths: map[int]map[int]int{
				1: {2: 2, 3: 2},
				2: {1: 2, 3: 2},
				3: {1: 2, 2: 2},
			},
			start:      1,
			end:        3,
			candidates: []models.Candidate{{CandidateID: 1}, {CandidateID: 2}, {CandidateID: 3}},
			want:       [][]int{{1, 2}, {2, 3}},
		},
		{
			name: "MediumCase",
			preferences: map[int]map[int]int{
				1: {2: 1, 3: 1},
				2: {1: 0, 3: 2},
				3: {1: 2, 2: 1},
			},
			strongestPaths: map[int]map[int]int{
				1: {2: 1, 3: 1},
				2: {1: 2, 3: 2},
				3: {1: 2, 2: 1},
			},
			start:      1,
			end:        3,
			candidates: []models.Candidate{{CandidateID: 1}, {CandidateID: 2}, {CandidateID: 3}},
			want:       [][]int{{1, 2}},
		},
		{
			name: "HardCase",
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
			start:      1,
			end:        5,
			candidates: []models.Candidate{{CandidateID: 1}, {CandidateID: 2}, {CandidateID: 3}, {CandidateID: 4}, {CandidateID: 5}},
			want:       [][]int{{3, 5}},
		},
		{
			name: "Example4", // https://electowiki.org/wiki/Schulze
			preferences: map[int]map[int]int{
				1: {2: 5, 3: 5, 4: 3},
				2: {1: 4, 3: 7, 4: 5},
				3: {1: 4, 2: 2, 4: 5},
				4: {1: 6, 2: 4, 3: 4},
			},
			strongestPaths: map[int]map[int]int{
				1: {2: 5, 3: 5, 4: 5},
				2: {1: 5, 3: 7, 4: 5},
				3: {1: 5, 2: 5, 4: 5},
				4: {1: 6, 2: 5, 3: 5},
			},
			start:      3,
			end:        2,
			candidates: []models.Candidate{{CandidateID: 1}, {CandidateID: 2}, {CandidateID: 3}, {CandidateID: 4}},
			want:       [][]int{{3, 4}, {1, 2}},
		},
		{
			name: "Example12", // https://arxiv.org/pdf/1804.02973
			preferences: map[int]map[int]int{
				1: {2: 20, 3: 24, 4: 32, 5: 16},
				2: {1: 25, 3: 26, 4: 23, 5: 18},
				3: {1: 21, 2: 19, 4: 30, 5: 14},
				4: {1: 13, 2: 22, 3: 15, 5: 28},
				5: {1: 29, 2: 27, 3: 31, 4: 17},
			},
			strongestPaths: map[int]map[int]int{
				1: {2: 27, 3: 28, 4: 32, 5: 28},
				2: {1: 26, 3: 26, 4: 26, 5: 26},
				3: {1: 28, 2: 27, 4: 30, 5: 28},
				4: {1: 28, 2: 27, 3: 28, 5: 28},
				5: {1: 29, 2: 27, 3: 31, 4: 30},
			},
			start:      1,
			end:        3,
			candidates: []models.Candidate{{CandidateID: 1}, {CandidateID: 2}, {CandidateID: 3}, {CandidateID: 4}, {CandidateID: 5}},
			want:       [][]int{{4, 5}},
		},
		{
			name: "NoPath",
			preferences: map[int]map[int]int{
				1: {2: 1, 3: 1},
				2: {1: 1, 3: 2},
				3: {1: 2, 2: 1},
			},
			strongestPaths: map[int]map[int]int{
				1: {2: 2, 3: 2},
				2: {1: 2, 3: 2},
				3: {1: 2, 2: 2},
			},
			start:      1,
			end:        2,
			candidates: []models.Candidate{{CandidateID: 1}, {CandidateID: 2}, {CandidateID: 3}},
			want:       [][]int{},
		},
		{
			name:           "NoCandidates",
			preferences:    map[int]map[int]int{},
			strongestPaths: map[int]map[int]int{},
			start:          1,
			end:            2,
			candidates:     []models.Candidate{},
			want:           [][]int{},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := s.findAllWeakestEdges(tt.preferences, tt.strongestPaths, tt.start, tt.end, tt.candidates); !assert.ElementsMatch(t, tt.want, got) {
				t.Errorf("Schulze.findAllWeakestEdges() =\n pass %v,\n want %v", got, tt.want)
			}
		})
	}
}

func TestSchulze_tieBreaker(t *testing.T) {
	t.Parallel()
	s := &Schulze{}
	tests := []struct {
		name             string
		potentialWinners []models.Candidate
		candidates       []models.Candidate
		preferences      map[int]map[int]int
		strongestPaths   map[int]map[int]int
		want             []models.Candidate
		wantErr          error
	}{
		{
			name: "HardCase", // https://arxiv.org/pdf/1804.02973
			potentialWinners: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
				{CandidateID: 4},
			},
			preferences: map[int]map[int]int{
				1: {2: 33, 3: 39, 4: 18},
				2: {1: 30, 3: 48, 4: 21},
				3: {1: 24, 2: 15, 4: 36},
				4: {1: 45, 2: 42, 3: 27},
			},
			strongestPaths: map[int]map[int]int{
				1: {2: 36, 3: 39, 4: 36},
				2: {1: 36, 3: 48, 4: 36},
				3: {1: 36, 2: 36, 4: 36},
				4: {1: 45, 2: 42, 3: 42},
			},
			want:    []models.Candidate{{CandidateID: 1}},
			wantErr: nil,
		},
		{
			name: "UnresolvableTie",
			potentialWinners: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
			},
			preferences: map[int]map[int]int{
				1: {2: 0, 3: 2, 4: 1},
				2: {1: 2, 3: 0, 4: 0},
				3: {1: 0, 2: 3, 4: 1},
				4: {1: 0, 2: 0, 3: 0},
			},
			strongestPaths: map[int]map[int]int{
				1: {2: 2, 3: 2, 4: 1},
				2: {1: 2, 3: 2, 4: 1},
				3: {1: 2, 2: 3, 4: 1},
				4: {1: 0, 2: 0, 3: 0},
			},
			want:    []models.Candidate{{CandidateID: 1}, {CandidateID: 2}},
			wantErr: nil,
		},
		{
			name: "Example4",
			potentialWinners: []models.Candidate{
				{CandidateID: 2},
				{CandidateID: 4},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
				{CandidateID: 4},
			},
			preferences: map[int]map[int]int{
				1: {2: 5, 3: 5, 4: 3},
				2: {1: 4, 3: 7, 4: 5},
				3: {1: 4, 2: 2, 4: 5},
				4: {1: 6, 2: 4, 3: 4},
			},
			strongestPaths: map[int]map[int]int{
				1: {2: 5, 3: 5, 4: 5},
				2: {1: 5, 3: 7, 4: 5},
				3: {1: 5, 2: 5, 4: 5},
				4: {1: 6, 2: 5, 3: 5},
			},
			want:    []models.Candidate{{CandidateID: 2}, {CandidateID: 4}},
			wantErr: nil,
		},
		{
			name: "Example12",
			potentialWinners: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 3},
			},
			candidates: []models.Candidate{
				{CandidateID: 1},
				{CandidateID: 2},
				{CandidateID: 3},
				{CandidateID: 4},
				{CandidateID: 5},
			},
			preferences: map[int]map[int]int{
				1: {2: 20, 3: 24, 4: 32, 5: 16},
				2: {1: 25, 3: 26, 4: 23, 5: 18},
				3: {1: 21, 2: 19, 4: 30, 5: 14},
				4: {1: 13, 2: 22, 3: 15, 5: 28},
				5: {1: 29, 2: 27, 3: 31, 4: 17},
			},
			strongestPaths: map[int]map[int]int{
				1: {2: 27, 3: 28, 4: 32, 5: 28},
				2: {1: 26, 3: 26, 4: 26, 5: 26},
				3: {1: 28, 2: 27, 4: 30, 5: 28},
				4: {1: 28, 2: 27, 3: 28, 5: 28},
				5: {1: 29, 2: 27, 3: 31, 4: 30},
			},
			want:    []models.Candidate{{CandidateID: 1}},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := s.tieBreaker(tt.potentialWinners, tt.candidates, tt.preferences, tt.strongestPaths)
			if tt.wantErr != nil {
				assert.Error(t, err, "Schulze.tieBreaker() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equalf(t, tt.want, got, "Schulze.tieBreaker() = %v, want %v", got, tt.want)
		})
	}
}
