package chain

import (
	"context"
	"fmt"
	"time"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"

	"github.com/jackc/pgx/v5"
)

func (vc *VoteChain) AddVote(ctx context.Context, telegramID int64, votes []int) error {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("chain.AddVote: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	delegate, err := vc.storage.GetDelegateByTelegramID(ctx, tx, telegramID)
	if err != nil {
		return fmt.Errorf("chain.AddVote: %w", err)
	}
	if delegate == nil {
		return fmt.Errorf("chain.AddVote: delegate not found")
	}

	vote := models.Vote{
		DelegateID:        delegate.DelegateID,
		CandidateRankings: votes,
		CreatedAt:         time.Now(),
	}

	currentVote, err := vc.storage.GetVoteByDelegateID(ctx, tx, delegate.DelegateID)
	if err != nil {
		return fmt.Errorf("chain.AddVote: %w", err)
	}
	if currentVote != nil {
		if err := vc.storage.UpdateVote(ctx, tx, vote); err != nil {
			return fmt.Errorf("chain.AddVote: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("chain.AddVote: can't commit transaction: %w", err)
		}
		return nil
	}

	if err = vc.storage.AddVote(ctx, tx, vote); err != nil {
		return fmt.Errorf("chain.AddVote: %w", err)
	}

	delegate.HasVoted = true
	if err = vc.storage.UpdateDelegate(ctx, tx, *delegate); err != nil {
		return fmt.Errorf("chain.AddVote: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("chain.AddVote: can't commit transaction: %w", err)
	}
	return nil

}

func (vc *VoteChain) GetAllVotes(ctx context.Context) ([]models.Vote, error) {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("chain.GetVotes: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	votes, err := vc.storage.GetAllVotes(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("chain.GetVotes: %w", err)
	}

	return votes, nil
}

func (vc *VoteChain) UpdateVote(ctx context.Context, vote models.Vote) error {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("chain.UpdateVote: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := vc.storage.UpdateVote(ctx, tx, vote); err != nil {
		return fmt.Errorf("chain.UpdateVote: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("chain.UpdateVote: can't commit transaction: %w", err)
	}
	return nil
}

func (vc *VoteChain) DeleteVoteByDelegateID(ctx context.Context, delegateID int) error {
	tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("chain.DeleteVote: can't start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	vote, err := vc.storage.GetVoteByDelegateID(ctx, tx, delegateID)
	if err != nil {
		return fmt.Errorf("chain.DeleteVote: %w", err)
	}
	if vote == nil {
		return fmt.Errorf("chain.DeleteVote: vote not found")
	}

	voteID := vote.ID

	if err := vc.storage.DeleteVote(ctx, tx, voteID); err != nil {
		return fmt.Errorf("chain.DeleteVote: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("chain.DeleteVote: can't commit transaction: %w", err)
	}
	return nil
}
