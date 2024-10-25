package schulze

import (
	"context"
	"fmt"
	"vote_system/internal/models"

	"github.com/sirupsen/logrus"
)

type Schulze struct {
	voteChain chain // цепочка для заимодействия с базой данных

	votes      []models.Vote      // список всех голосов
	candidates []models.Candidate // список всех кандидатов

	votesByCourse      map[string][]models.Vote      // список голосов по курсам (курс -> бюллетени)
	candidatesByCourse map[string][]models.Candidate // список кандидатов по курсам (курс -> кандидаты)

	// results map[string]models.Result
}

func NewSchulze(voteChain chain) *Schulze {
	return &Schulze{
		voteChain: voteChain,

		// TODO нужно ли инициализировать?
		// votes:      []models.Vote{},
		// candidates: []models.Candidate{},

		votesByCourse:      make(map[string][]models.Vote),
		candidatesByCourse: make(map[string][]models.Candidate),

		// results: make(map[string]models.Result),
	}
}

type chain interface {
	GetAllEligibleCandidates(ctx context.Context) ([]models.Candidate, error)
	GetAllVotes(ctx context.Context) ([]models.Vote, error)
	AddResult(ctx context.Context, result models.Result) error
	GetAllResults(ctx context.Context) ([]models.Result, error)
	GetCandidateByCandidateID(ctx context.Context, candidateID int) (*models.Candidate, error)
}

func (s *Schulze) SetCandidates() error {
	candidates, err := s.voteChain.GetAllEligibleCandidates(context.Background())
	if err != nil {
		return fmt.Errorf("SetCandidates: %w", err)
	}
	s.candidates = candidates
	logrus.Debug(s.candidates)
	return nil
}

func (s *Schulze) SetVotes() error {
	votes, err := s.voteChain.GetAllVotes(context.Background())
	if err != nil {
		return fmt.Errorf("SetVotes: %w", err)
	}
	// copy не работает
	s.votes = votes
	logrus.Debug(s.votes)
	return nil
}

func (s *Schulze) SetCandidatesByCourse() error {
	// Проверяем, что кандидаты уже установлены
	if len(s.candidates) == 0 {
		return fmt.Errorf("candidates not set")
	}
	// Очищаем поле
	s.candidatesByCourse = make(map[string][]models.Candidate)
	// Фильтрация кандидатов по курсам
	for _, candidate := range s.candidates {
		s.candidatesByCourse[candidate.Course] = append(s.candidatesByCourse[candidate.Course], candidate)
	}
	logrus.Debug(s.candidatesByCourse)
	return nil
}

func (s *Schulze) SetVotesByCourse() error {
	// Проверяем, что кандидаты по курсам уже установлены
	if len(s.candidatesByCourse) == 0 {
		return fmt.Errorf("candidates by course not set")
	}
	// Очищаем поле
	s.votesByCourse = make(map[string][]models.Vote)
	// Фильтрация голосов для каждого курса
	for course, candidates := range s.candidatesByCourse {
		candidateMap := make(map[int]bool)
		for _, candidate := range candidates {
			candidateMap[candidate.CandidateID] = true
		}
		for _, vote := range s.votes {
			var filteredRankings []int
			for _, candidateID := range vote.CandidateRankings {
				if candidateMap[candidateID] {
					filteredRankings = append(filteredRankings, candidateID)
				}
			}
			// Если ранжировка не пустая, добавляем её к голосам курса
			if len(filteredRankings) > 0 {
				vote.CandidateRankings = filteredRankings
				s.votesByCourse[course] = append(s.votesByCourse[course], vote)
			}
		}
	}
	logrus.Debug(s.votesByCourse)
	return nil
}
