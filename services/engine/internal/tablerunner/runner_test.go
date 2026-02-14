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

	if result.FinalState.Phase != domain.HandPhaseComplete {
		t.Fatalf("expected phase %q, got %q", domain.HandPhaseComplete, result.FinalState.Phase)
	}
	if result.FinalState.Pot != 0 {
		t.Fatalf("expected pot to be fully settled, got %d", result.FinalState.Pot)
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

func TestRunHand_InvokesOnHandStartOnce(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	callCount := 0
	var gotInput RunHandInput
	var gotInitial domain.HandState

	runner := New(newScriptedProvider(
		actionCall(t),
		actionCheck(t),
		actionCheck(t),
		actionCheck(t),
		actionCheck(t),
		actionCheck(t),
		actionCheck(t),
		actionCheck(t),
	), RunnerConfig{
		OnHandStart: func(input RunHandInput, initial domain.HandState) {
			callCount++
			gotInput = input
			gotInitial = initial
		},
	})

	input := RunHandInput{
		TableID:    "table-1",
		HandNo:     7,
		ButtonSeat: mustSeatNo(t, cfg, 1),
		Seats:      activeSeats(t, cfg, 1, 2),
		Config:     cfg,
	}
	_, err := runner.RunHand(context.Background(), input)
	if err != nil {
		t.Fatalf("RunHand failed: %v", err)
	}

	if callCount != 1 {
		t.Fatalf("expected OnHandStart to be called once, got %d", callCount)
	}
	if gotInput.HandNo != input.HandNo {
		t.Fatalf("expected hand no %d, got %d", input.HandNo, gotInput.HandNo)
	}
	if gotInitial.Phase != domain.HandPhaseBetting {
		t.Fatalf("expected initial phase %q, got %q", domain.HandPhaseBetting, gotInitial.Phase)
	}
}

func TestRunHand_InvokesOnActionForNormalActions(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	actionCalls := 0
	fallbackCalls := 0

	runner := New(newScriptedProvider(actionCall(t), actionFold(t)), RunnerConfig{
		OnAction: func(_ uint64, _ domain.HandState, _ domain.Action, isFallback bool) {
			if isFallback {
				fallbackCalls++
				return
			}
			actionCalls++
		},
	})

	_, err := runner.RunHand(context.Background(), RunHandInput{
		TableID:    "table-1",
		HandNo:     1,
		ButtonSeat: mustSeatNo(t, cfg, 1),
		Seats:      activeSeats(t, cfg, 1, 2),
		Config:     cfg,
	})
	if err != nil {
		t.Fatalf("RunHand failed: %v", err)
	}

	if actionCalls != 2 {
		t.Fatalf("expected 2 normal action callbacks, got %d", actionCalls)
	}
	if fallbackCalls != 0 {
		t.Fatalf("expected no fallback callbacks, got %d", fallbackCalls)
	}
}

func TestRunHand_InvokesOnActionForFallbackActions(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	actionCalls := 0
	fallbackCalls := 0
	var fallbackAction domain.Action

	runner := New(newScriptedProvider(scriptedStep{err: errors.New("provider failed")}), RunnerConfig{
		OnAction: func(_ uint64, _ domain.HandState, action domain.Action, isFallback bool) {
			if isFallback {
				fallbackCalls++
				fallbackAction = action
				return
			}
			actionCalls++
		},
	})

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
	if actionCalls != 0 {
		t.Fatalf("expected no normal callbacks, got %d", actionCalls)
	}
	if fallbackCalls != 1 {
		t.Fatalf("expected exactly one fallback callback, got %d", fallbackCalls)
	}
	if fallbackAction.Kind != domain.ActionFold && fallbackAction.Kind != domain.ActionCheck {
		t.Fatalf("expected fallback check/fold action, got %s", fallbackAction.Kind)
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

func TestRunTable_InvokesOnHandCompleteCallback(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	var completed []uint64
	runner := New(newScriptedProvider(), RunnerConfig{
		OnHandComplete: func(summary HandSummary) {
			completed = append(completed, summary.HandNo)
		},
	})

	_, err := runner.RunTable(context.Background(), RunTableInput{
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

	if len(completed) != 3 {
		t.Fatalf("expected callback for 3 hands, got %d", len(completed))
	}
	for i, handNo := range completed {
		want := uint64(i + 1)
		if handNo != want {
			t.Fatalf("callback hand index %d: want %d got %d", i, want, handNo)
		}
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
	seats[1].Status = domain.SeatStatusSittingOut

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
		if summary.FinalState.Pot != 0 {
			t.Fatalf("hand %d expected settled pot 0, got %d", i+1, summary.FinalState.Pot)
		}
		if len(summary.FinalState.ShowdownAwards) == 0 {
			t.Fatalf("hand %d expected showdown awards to be recorded", i+1)
		}
		assertUniqueCards(t, summary.FinalState)
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

func actionFold(t *testing.T) scriptedStep {
	t.Helper()
	a := mustAction(t, domain.ActionFold, nil)
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

func assertUniqueCards(t *testing.T, state domain.HandState) {
	t.Helper()

	seen := map[string]struct{}{}
	for _, card := range state.Board {
		key := cardSignature(card)
		if _, ok := seen[key]; ok {
			t.Fatalf("duplicate board card %s", key)
		}
		seen[key] = struct{}{}
	}
	for _, seatCards := range state.HoleCards {
		for _, card := range seatCards.Cards {
			key := cardSignature(card)
			if _, ok := seen[key]; ok {
				t.Fatalf("duplicate hole/board card %s", key)
			}
			seen[key] = struct{}{}
		}
	}
}

func cardSignature(card domain.Card) string {
	return string(card.Suit) + "-" + string(rune(card.Rank))
}
