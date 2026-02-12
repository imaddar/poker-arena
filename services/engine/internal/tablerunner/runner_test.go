package tablerunner

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	"github.com/imaddar/poker-arena/services/engine/internal/statemachine"
)

func TestRunHand_CompletesWithScriptedActions(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	runner := New(newScriptedProvider(
		actionCall(t),
		actionCheck(t),
		actionCheck(t),
		actionCheck(t),
		actionCheck(t),
		actionCheck(t),
		actionCheck(t),
		actionCheck(t),
	), RunnerConfig{})

	result, err := runner.RunHand(context.Background(), RunHandInput{
		TableID:    "table-1",
		HandNo:     1,
		ButtonSeat: mustSeatNo(t, cfg, 1),
		Seats:      activeSeats(t, cfg, 1, 2),
		Config:     cfg,
	})
	if err != nil {
		t.Fatalf("RunHand failed: %v", err)
	}

	if result.FinalState.Phase != domain.HandPhaseShowdown && result.FinalState.Phase != domain.HandPhaseComplete {
		t.Fatalf("expected terminal phase, got %q", result.FinalState.Phase)
	}
	if result.ActionCount == 0 {
		t.Fatal("expected action count > 0")
	}
	if chipTotal(result.FinalState) != cfg.StartingStack*2 {
		t.Fatalf("chip conservation failed: got %d", chipTotal(result.FinalState))
	}
}

func TestRunHand_UsesFallbackWhenProviderErrors(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	runner := New(newScriptedProvider(scriptedStep{err: errors.New("boom")}), RunnerConfig{})

	result, err := runner.RunHand(context.Background(), RunHandInput{
		TableID:    "table-1",
		HandNo:     1,
		ButtonSeat: mustSeatNo(t, cfg, 1),
		Seats:      activeSeats(t, cfg, 1, 2),
		Config:     cfg,
	})
	if err != nil {
		t.Fatalf("RunHand failed: %v", err)
	}
	if result.FallbackCount == 0 {
		t.Fatal("expected fallback count > 0")
	}
}

func TestRunHand_UsesFallbackWhenProviderReturnsIllegalAction(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	runner := New(newScriptedProvider(actionCheck(t)), RunnerConfig{})

	result, err := runner.RunHand(context.Background(), RunHandInput{
		TableID:    "table-1",
		HandNo:     1,
		ButtonSeat: mustSeatNo(t, cfg, 1),
		Seats:      activeSeats(t, cfg, 1, 2),
		Config:     cfg,
	})
	if err != nil {
		t.Fatalf("RunHand failed: %v", err)
	}
	if result.FallbackCount == 0 {
		t.Fatal("expected fallback count > 0")
	}
	if result.FinalState.Phase != domain.HandPhaseComplete {
		t.Fatalf("expected complete phase after fallback fold, got %q", result.FinalState.Phase)
	}
}

func TestRunHand_StopsOnActionLimit(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	runner := New(newScriptedProvider(
		actionCall(t),
		actionCheck(t),
		actionCheck(t),
		actionCheck(t),
	), RunnerConfig{MaxActionsPerHand: 2})

	_, err := runner.RunHand(context.Background(), RunHandInput{
		TableID:    "table-1",
		HandNo:     1,
		ButtonSeat: mustSeatNo(t, cfg, 1),
		Seats:      activeSeats(t, cfg, 1, 2),
		Config:     cfg,
	})
	if !errors.Is(err, ErrActionLimitExceeded) {
		t.Fatalf("expected ErrActionLimitExceeded, got %v", err)
	}
}

func TestRunHand_RespectsContextCancellation(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	runner := New(newScriptedProvider(actionCall(t)), RunnerConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runner.RunHand(ctx, RunHandInput{
		TableID:    "table-1",
		HandNo:     1,
		ButtonSeat: mustSeatNo(t, cfg, 1),
		Seats:      activeSeats(t, cfg, 1, 2),
		Config:     cfg,
	})
	if !errors.Is(err, ErrContextCancelled) {
		t.Fatalf("expected ErrContextCancelled, got %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRunHand_PropagatesStartNewHandValidationError(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	seats := activeSeats(t, cfg, 1, 2)
	seats[0].Status = domain.SeatStatusSittingOut
	seats[1].Status = domain.SeatStatusSittingOut

	runner := New(newScriptedProvider(), RunnerConfig{})
	_, err := runner.RunHand(context.Background(), RunHandInput{
		TableID:    "table-1",
		HandNo:     1,
		ButtonSeat: mustSeatNo(t, cfg, 1),
		Seats:      seats,
		Config:     cfg,
	})
	if !errors.Is(err, statemachine.ErrNoActiveSeats) {
		t.Fatalf("expected statemachine.ErrNoActiveSeats, got %v", err)
	}
}

type scriptedProvider struct {
	steps []scriptedStep
	i     int
}

type scriptedStep struct {
	action domain.Action
	err    error
}

func newScriptedProvider(steps ...scriptedStep) *scriptedProvider {
	return &scriptedProvider{steps: steps}
}

func (p *scriptedProvider) NextAction(_ context.Context, _ domain.HandState) (domain.Action, error) {
	if p.i >= len(p.steps) {
		return domain.Action{}, fmt.Errorf("provider script exhausted at index %d", p.i)
	}
	step := p.steps[p.i]
	p.i++
	return step.action, step.err
}

func actionCall(t *testing.T) scriptedStep {
	t.Helper()
	a := mustAction(t, domain.ActionCall, nil)
	return scriptedStep{action: a}
}

func actionCheck(t *testing.T) scriptedStep {
	t.Helper()
	a := mustAction(t, domain.ActionCheck, nil)
	return scriptedStep{action: a}
}

func mustAction(t *testing.T, kind domain.ActionKind, amount *uint32) domain.Action {
	t.Helper()
	a, err := domain.NewAction(kind, amount)
	if err != nil {
		t.Fatalf("NewAction failed: %v", err)
	}
	return a
}

func activeSeats(t *testing.T, cfg domain.TableConfig, seatNos ...uint8) []domain.SeatState {
	t.Helper()
	seats := make([]domain.SeatState, 0, len(seatNos))
	for _, n := range seatNos {
		seats = append(seats, domain.NewSeatState(mustSeatNo(t, cfg, n), cfg.StartingStack))
	}
	return seats
}

func mustSeatNo(t *testing.T, cfg domain.TableConfig, n uint8) domain.SeatNo {
	t.Helper()
	seatNo, err := domain.NewSeatNo(n, cfg.MaxSeats)
	if err != nil {
		t.Fatalf("NewSeatNo failed: %v", err)
	}
	return seatNo
}

func chipTotal(state domain.HandState) uint32 {
	total := state.Pot
	for _, seat := range state.Seats {
		total += seat.Stack
	}
	return total
}
