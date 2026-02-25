package api

import (
	"context"
	"testing"
	"time"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

func TestHumanActionHub_WaitForActionReceivesSubmittedAction(t *testing.T) {
	t.Parallel()

	hub := NewHumanActionHub()
	state := domain.HandState{
		TableID:    "table-1",
		HandID:     "hand-1",
		ActingSeat: 1,
		CurrentBet: 100,
		MinRaiseTo: 200,
		Board: []domain.Card{
			{Rank: 14, Suit: domain.SuitSpades},
		},
		Seats: []domain.SeatState{
			{SeatNo: 1, Stack: 1000, CommittedInRound: 0, Status: domain.SeatStatusActive},
			{SeatNo: 2, Stack: 1000, CommittedInRound: 100, Status: domain.SeatStatusActive},
		},
		HoleCards: []domain.SeatCards{
			{SeatNo: 1, Cards: []domain.Card{{Rank: 10, Suit: domain.SuitClubs}, {Rank: 11, Suit: domain.SuitHearts}}},
		},
	}

	resultCh := make(chan domain.Action, 1)
	errCh := make(chan error, 1)
	go func() {
		action, err := hub.WaitForAction(context.Background(), state, 500)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- action
	}()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if pending, ok := hub.GetPending("table-1", 1); ok {
			if pending.HandID != "hand-1" {
				t.Fatalf("expected pending hand-1, got %s", pending.HandID)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := hub.SubmitAction("table-1", HumanActionRequest{
		SeatNo: 1,
		Action: "call",
	}); err != nil {
		t.Fatalf("SubmitAction failed: %v", err)
	}

	select {
	case action := <-resultCh:
		if action.Kind != domain.ActionCall {
			t.Fatalf("expected call, got %s", action.Kind)
		}
	case err := <-errCh:
		t.Fatalf("WaitForAction returned error: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for action result")
	}
}

func TestHumanActionHub_WaitForActionTimesOut(t *testing.T) {
	t.Parallel()

	hub := NewHumanActionHub()
	state := domain.HandState{
		TableID:    "table-timeout",
		HandID:     "hand-timeout",
		ActingSeat: 1,
		Seats: []domain.SeatState{
			{SeatNo: 1, Stack: 1000, Status: domain.SeatStatusActive},
		},
	}

	_, err := hub.WaitForAction(context.Background(), state, 50)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestHumanActionHub_SubmitIllegalActionRejected(t *testing.T) {
	t.Parallel()

	hub := NewHumanActionHub()
	state := domain.HandState{
		TableID:    "table-illegal",
		HandID:     "hand-illegal",
		ActingSeat: 1,
		CurrentBet: 100,
		Seats: []domain.SeatState{
			{SeatNo: 1, Stack: 1000, CommittedInRound: 0, Status: domain.SeatStatusActive},
			{SeatNo: 2, Stack: 1000, CommittedInRound: 100, Status: domain.SeatStatusActive},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_, _ = hub.WaitForAction(ctx, state, 500)
	}()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, ok := hub.GetPending("table-illegal", 1); ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := hub.SubmitAction("table-illegal", HumanActionRequest{
		SeatNo: 1,
		Action: "check",
	}); err == nil {
		t.Fatal("expected illegal action rejection when to_call > 0")
	}
}
