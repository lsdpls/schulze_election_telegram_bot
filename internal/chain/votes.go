package chain

import (
	"context"
	"fmt"
	"time"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"

	"github.com/jackc/pgx/v5"
)

// AddVote записывает бюллетень делегата: первый голос вставляется, повторный перезаписывает прежний
// (один запрос INSERT … ON CONFLICT (delegate_id) DO UPDATE); has_voted ставится при первом голосе
func (vc *VoteChain) AddVote(ctx context.Context, telegramID int64, votes []int) error {
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		delegate, err := vc.storage.GetDelegateByTelegramID(ctx, tx, telegramID)
		if err != nil {
			return err
		}
		if delegate == nil {
			return fmt.Errorf("delegate not found")
		}

		vote := models.Vote{
			DelegateID:        delegate.DelegateID,
			CandidateRankings: votes,
			CreatedAt:         time.Now(),
		}
		if err := vc.storage.UpsertVote(ctx, tx, vote); err != nil {
			return err
		}

		if !delegate.HasVoted {
			delegate.HasVoted = true
			if err := vc.storage.UpdateDelegate(ctx, tx, *delegate); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("chain.AddVote: %w", err)
	}
	return nil
}

func (vc *VoteChain) GetAllVotes(ctx context.Context) ([]models.Vote, error) {
	var votes []models.Vote
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		votes, err = vc.storage.GetAllVotes(ctx, tx)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("chain.GetVotes: %w", err)
	}
	return votes, nil
}

func (vc *VoteChain) UpdateVote(ctx context.Context, vote models.Vote) error {
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		return vc.storage.UpdateVote(ctx, tx, vote)
	})
	if err != nil {
		return fmt.Errorf("chain.UpdateVote: %w", err)
	}
	return nil
}

func (vc *VoteChain) DeleteVoteByDelegateID(ctx context.Context, delegateID int) error {
	err := vc.inTx(ctx, func(tx pgx.Tx) error {
		vote, err := vc.storage.GetVoteByDelegateID(ctx, tx, delegateID)
		if err != nil {
			return err
		}
		if vote == nil {
			return fmt.Errorf("vote not found")
		}
		return vc.storage.DeleteVote(ctx, tx, vote.ID)
	})
	if err != nil {
		return fmt.Errorf("chain.DeleteVote: %w", err)
	}
	return nil
}
