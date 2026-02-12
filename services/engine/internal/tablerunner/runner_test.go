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

func TestRunTable_CompletesRequestedHands(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	runner := New(newScriptedProvider(), RunnerConfig{})

	result, err := runner.RunTable(context.Background(), RunTableInput{
		TableID:      "table-1",
		StartingHand: 1,
		HandsToRun:   3,
		ButtonSeat:   mustSeatNo(t, cfg, 1),
		Seats:        activeSeats(t, cfg, 1, 2),
		Config:       cfg,
	})
	if err != nil {
		t.Fatalf("RunTable failed: %v", err)
	}

	if result.HandsCompleted != 3 {
		t.Fatalf("expected 3 hands completed, got %d", result.HandsCompleted)
	}
	if len(result.HandSummaries) != 3 {
		t.Fatalf("expected 3 hand summaries, got %d", len(result.HandSummaries))
	}
}

func TestRunTable_RotatesButtonAcrossHands(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	runner := New(newScriptedProvider(), RunnerConfig{})

	result, err := runner.RunTable(context.Background(), RunTableInput{
		TableID:      "table-1",
		StartingHand: 1,
		HandsToRun:   3,
		ButtonSeat:   mustSeatNo(t, cfg, 1),
		Seats:        activeSeats(t, cfg, 1, 2, 3),
		Config:       cfg,
	})
	if err != nil {
		t.Fatalf("RunTable failed: %v", err)
	}

	if result.FinalButton != mustSeatNo(t, cfg, 1) {
		t.Fatalf("expected final button seat 1, got %d", result.FinalButton)
	}
}

func TestRunTable_CarriesForwardSeatStacks(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	runner := New(newScriptedProvider(), RunnerConfig{})

	initialSeats := activeSeats(t, cfg, 1, 2)
	initialStacks := map[domain.SeatNo]uint32{
		initialSeats[0].SeatNo: initialSeats[0].Stack,
		initialSeats[1].SeatNo: initialSeats[1].Stack,
	}

	result, err := runner.RunTable(context.Background(), RunTableInput{
		TableID:      "table-1",
		StartingHand: 1,
		HandsToRun:   2,
		ButtonSeat:   mustSeatNo(t, cfg, 1),
		Seats:        initialSeats,
		Config:       cfg,
	})
	if err != nil {
		t.Fatalf("RunTable failed: %v", err)
	}

	changed := false
	for _, summary := range result.HandSummaries {
		for _, seat := range summary.FinalState.Seats {
			if seat.Stack != initialStacks[seat.SeatNo] {
				changed = true
				break
			}
		}
		if changed {
			break
		}
	}
	if !changed {
		t.Fatal("expected seat stacks to change during table run")
	}
}

func TestRunTable_StopsWhenTooFewActiveSeats(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	seats := activeSeats(t, cfg, 1, 2)
	seats[0].Stack = 50

	runner := New(newScriptedProvider(), RunnerConfig{})
	_, err := runner.RunTable(context.Background(), RunTableInput{
		TableID:      "table-1",
		StartingHand: 1,
		HandsToRun:   2,
		ButtonSeat:   mustSeatNo(t, cfg, 1),
		Seats:        seats,
		Config:       cfg,
	})
	if !errors.Is(err, ErrInsufficientActiveSeats) {
		t.Fatalf("expected ErrInsufficientActiveSeats, got %v", err)
	}
}

func TestRunTable_RejectsInvalidHandsToRun(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	runner := New(newScriptedProvider(), RunnerConfig{})
	_, err := runner.RunTable(context.Background(), RunTableInput{
		TableID:      "table-1",
		StartingHand: 1,
		HandsToRun:   0,
		ButtonSeat:   mustSeatNo(t, cfg, 1),
		Seats:        activeSeats(t, cfg, 1, 2),
		Config:       cfg,
	})
	if !errors.Is(err, ErrInvalidHandsToRun) {
		t.Fatalf("expected ErrInvalidHandsToRun, got %v", err)
	}
}

func TestRunTable_RespectsContextCancellation(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	runner := New(newScriptedProvider(), RunnerConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runner.RunTable(ctx, RunTableInput{
		TableID:      "table-1",
		StartingHand: 1,
		HandsToRun:   2,
		ButtonSeat:   mustSeatNo(t, cfg, 1),
		Seats:        activeSeats(t, cfg, 1, 2),
		Config:       cfg,
	})
	if !errors.Is(err, ErrContextCancelled) {
		t.Fatalf("expected ErrContextCancelled, got %v", err)
	}
}

func TestRunTable_PreservesChipConservationAcrossHands(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	runner := New(newScriptedProvider(), RunnerConfig{})

	result, err := runner.RunTable(context.Background(), RunTableInput{
		TableID:      "table-1",
		StartingHand: 1,
		HandsToRun:   3,
		ButtonSeat:   mustSeatNo(t, cfg, 1),
		Seats:        activeSeats(t, cfg, 1, 2),
		Config:       cfg,
	})
	if err != nil {
		t.Fatalf("RunTable failed: %v", err)
	}

	total := cfg.StartingStack * 2
	for i, summary := range result.HandSummaries {
		got := chipTotal(summary.FinalState)
		if got != total {
			t.Fatalf("hand %d chip conservation failed: want %d got %d", i+1, total, got)
		}
	}
}

func TestRunTable_CompletesOneHundredHandsWithDeterministicProvider(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	runner := New(&deterministicProvider{}, RunnerConfig{})

	result, err := runner.RunTable(context.Background(), RunTableInput{
		TableID:      "table-1",
		StartingHand: 1,
		HandsToRun:   100,
		ButtonSeat:   mustSeatNo(t, cfg, 1),
		Seats:        activeSeats(t, cfg, 1, 2),
		Config:       cfg,
	})
	if err != nil {
		t.Fatalf("RunTable failed: %v", err)
	}

	if result.HandsCompleted != 100 {
		t.Fatalf("expected 100 hands completed, got %d", result.HandsCompleted)
	}
	if len(result.HandSummaries) != 100 {
		t.Fatalf("expected 100 hand summaries, got %d", len(result.HandSummaries))
	}
}

func TestRunTable_OneHundredHandsPreserveChipConservation(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	runner := New(&deterministicProvider{}, RunnerConfig{})

	result, err := runner.RunTable(context.Background(), RunTableInput{
		TableID:      "table-1",
		StartingHand: 1,
		HandsToRun:   100,
		ButtonSeat:   mustSeatNo(t, cfg, 1),
		Seats:        activeSeats(t, cfg, 1, 2),
		Config:       cfg,
	})
	if err != nil {
		t.Fatalf("RunTable failed: %v", err)
	}

	initialTotal := cfg.StartingStack * 2
	for i, summary := range result.HandSummaries {
		got := chipTotal(summary.FinalState)
		if got != initialTotal {
			t.Fatalf("hand %d chip conservation failed: want %d got %d", i+1, initialTotal, got)
		}
	}

	final := uint32(0)
	for _, seat := range result.FinalSeats {
		final += seat.Stack
	}
	if final != initialTotal {
		t.Fatalf("final seat chip total failed: want %d got %d", initialTotal, final)
	}
}

type scriptedProvider struct {
	steps []scriptedStep
	i     int
}

type deterministicProvider struct{}

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

func (p *deterministicProvider) NextAction(_ context.Context, state domain.HandState) (domain.Action, error) {
	var actingSeat *domain.SeatState
	for i := range state.Seats {
		if state.Seats[i].SeatNo == state.ActingSeat {
			actingSeat = &state.Seats[i]
			break
		}
	}
	if actingSeat == nil {
		return domain.Action{}, fmt.Errorf("acting seat %d not found", state.ActingSeat)
	}

	if state.CurrentBet > actingSeat.CommittedInRound {
		action, err := domain.NewAction(domain.ActionCall, nil)
		if err != nil {
			return domain.Action{}, err
		}
		return action, nil
	}

	action, err := domain.NewAction(domain.ActionCheck, nil)
	if err != nil {
		return domain.Action{}, err
	}
	return action, nil
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
