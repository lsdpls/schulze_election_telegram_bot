package chain

import (
	"context"
	"fmt"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"

	"github.com/jackc/pgx/v5"
)

func (vc *VoteChain) AddCandidate(ctx context.Context, candidate models.Candidate) error {
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		candidateDB, err := vc.storage.GetCandidateByCandidateID(ctx, tx, candidate.CandidateID)
		if err != nil {
			return err
		}
		if candidateDB != nil {
			return fmt.Errorf("candidate already exists")
		}
		return vc.storage.AddCandidate(ctx, tx, candidate)
	})
	if err != nil {
		return fmt.Errorf("chain.AddCandidate: %w", err)
	}
	return nil
}

// DeleteCandidate удаляет строку кандидата из БД. Бот его не вызывает: /delete_candidate снимает кандидата
// мягко через BanCandidate (is_eligible = false), чтобы бюллетени и результаты не ссылались на пропавший ID
func (vc *VoteChain) DeleteCandidate(ctx context.Context, candidateID int) error {
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		candidate, err := vc.storage.GetCandidateByCandidateID(ctx, tx, candidateID)
		if err != nil {
			return err
		}
		if candidate == nil {
			return fmt.Errorf("candidate not found")
		}
		return vc.storage.DeleteCandidate(ctx, tx, candidateID)
	})
	if err != nil {
		return fmt.Errorf("chain.DeleteCandidate: %w", err)
	}
	return nil
}

func (vc *VoteChain) GetAllCandidates(ctx context.Context) ([]models.Candidate, error) {
	var candidates []models.Candidate
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		candidates, err = vc.storage.GetAllCandidates(ctx, tx)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("chain.GetAllCandidates: %w", err)
	}
	return candidates, nil
}

func (vc *VoteChain) GetAllEligibleCandidates(ctx context.Context) ([]models.Candidate, error) {
	var candidates []models.Candidate
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		candidates, err = vc.storage.GetAllEligibleCandidates(ctx, tx)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("chain.GetAllEligibleCandidates: %w", err)
	}
	return candidates, nil
}

func (vc *VoteChain) GetCandidateByCandidateID(ctx context.Context, candidateID int) (*models.Candidate, error) {
	var candidate *models.Candidate
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		candidate, err = vc.storage.GetCandidateByCandidateID(ctx, tx, candidateID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("chain.GetCandidateByCandidateID: %w", err)
	}
	return candidate, nil
}

// BanCandidate снимает кандидата с выборов: is_eligible = false, строка и бюллетени с его ID остаются;
// при подсчёте кандидат не учитывается
func (vc *VoteChain) BanCandidate(ctx context.Context, candidateID int) error {
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		candidate, err := vc.storage.GetCandidateByCandidateID(ctx, tx, candidateID)
		if err != nil {
			return err
		}
		if candidate == nil {
			return fmt.Errorf("candidate not found")
		}
		candidate.IsEligible = false
		return vc.storage.UpdateCandidate(ctx, tx, *candidate)
	})
	if err != nil {
		return fmt.Errorf("chain.BanCandidate: %w", err)
	}
	return nil
}
