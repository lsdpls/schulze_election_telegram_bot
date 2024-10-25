package chain

import (
	"context"
	"fmt"
	"vote_system/internal/models"

	"github.com/jackc/pgx/v5"
)

func (vc *VoteChain) AddCandidate(ctx context.Context, candidate models.Candidate) error {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("chain.AddCandidate: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	candidateDB, err := vc.storage.GetCandidateByCandidateID(ctx, tx, candidate.CandidateID)
	if err != nil {
		return fmt.Errorf("chain.AddCandidate: %w", err)
	}
	if candidateDB != nil {
		return fmt.Errorf("chain.AddCandidate: candidate already exists")
	}

	if err := vc.storage.AddCandidate(ctx, tx, candidate); err != nil {
		return fmt.Errorf("chain.AddCandidate: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("chain.AddCandidate: can't commit transaction: %w", err)
	}
	return nil
}

func (vc *VoteChain) DeleteCandidate(ctx context.Context, candidateID int) error {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("chain.DeleteCandidate: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	candidate, err := vc.storage.GetCandidateByCandidateID(ctx, tx, candidateID)
	if err != nil {
		return fmt.Errorf("chain.DeleteCandidate: %w", err)
	}
	if candidate == nil {
		return fmt.Errorf("chain.DeleteCandidate: candidate not found")
	}

	if err := vc.storage.DeleteCandidate(ctx, tx, candidateID); err != nil {
		return fmt.Errorf("chain.DeleteCandidate: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("chain.DeleteCandidate: can't commit transaction: %w", err)
	}
	return nil
}

func (vc *VoteChain) GetAllCandidates(ctx context.Context) ([]models.Candidate, error) {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("chain.GetAllCandidates: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	candidates, err := vc.storage.GetAllCandidates(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("chain.GetAllCandidates: %w", err)
	}
	return candidates, nil
}

func (vc *VoteChain) GetAllEligibleCandidates(ctx context.Context) ([]models.Candidate, error) {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("chain.GetAllEligibleCandidates: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	candidates, err := vc.storage.GetAllEligibleCandidates(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("chain.GetAllEligibleCandidates: %w", err)
	}
	return candidates, nil
}

func (vc *VoteChain) GetCandidateByCandidateID(ctx context.Context, candidateID int) (*models.Candidate, error) {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("chain.GetCandidateByCandidateID: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	candidate, err := vc.storage.GetCandidateByCandidateID(ctx, tx, candidateID)
	if err != nil {
		return nil, fmt.Errorf("chain.GetCandidateByCandidateID: %w", err)
	}
	return candidate, nil
}

// TODO разобраться, что здесь происходит
func (vc *VoteChain) BanCandidate(ctx context.Context, candidateID int) error {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("chain.BanCandidate: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	candidate, err := vc.storage.GetCandidateByCandidateID(ctx, tx, candidateID)
	if err != nil {
		return fmt.Errorf("chain.BanCandidate: %w", err)
	}
	if candidate == nil {
		return fmt.Errorf("chain.BanCandidate: candidate not found")
	}

	candidate.IsEligible = false
	if err := vc.storage.UpdateCandidate(ctx, tx, *candidate); err != nil {
		return fmt.Errorf("chain.BanCandidate: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("chain.BanCandidate: can't commit transaction: %w", err)
	}
	return nil
}
