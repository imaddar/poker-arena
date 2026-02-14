package statemachine

import (
	"errors"
	"testing"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	"github.com/imaddar/poker-arena/services/engine/internal/rules"
)

func TestStartNewHandInitializesPreflopAndBlinds(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	seats := mustSeats(t, cfg, 1, 2, 3, 4)
	button := mustSeatNo(t, cfg, 1)

	state, err := StartNewHand(StartNewHandInput{
		TableID:    "table-1",
		HandNo:     1,
		Seats:      seats,
		ButtonSeat: button,
		Config:     cfg,
	})
	if err != nil {
		t.Fatalf("StartNewHand failed: %v", err)
	}

	if state.Phase != domain.HandPhaseBetting {
		t.Fatalf("expected phase %q, got %q", domain.HandPhaseBetting, state.Phase)
	}
	if state.Street != domain.StreetPreflop {
		t.Fatalf("expected street %q, got %q", domain.StreetPreflop, state.Street)
	}
	if state.Pot != cfg.SmallBlind+cfg.BigBlind {
		t.Fatalf("expected pot %d, got %d", cfg.SmallBlind+cfg.BigBlind, state.Pot)
	}

	sb := findSeat(t, state, mustSeatNo(t, cfg, 2))
	bb := findSeat(t, state, mustSeatNo(t, cfg, 3))
	utg := mustSeatNo(t, cfg, 4)

	if sb.CommittedInRound != cfg.SmallBlind {
		t.Fatalf("expected sb committed %d, got %d", cfg.SmallBlind, sb.CommittedInRound)
	}
	if bb.CommittedInRound != cfg.BigBlind {
		t.Fatalf("expected bb committed %d, got %d", cfg.BigBlind, bb.CommittedInRound)
	}
	if state.ActingSeat != utg {
		t.Fatalf("expected acting seat %d, got %d", utg, state.ActingSeat)
	}
	if len(state.HoleCards) != 4 {
		t.Fatalf("expected hole cards for 4 seats, got %d", len(state.HoleCards))
	}
}

func TestStartNewHandHeadsUpUsesButtonAsSmallBlindAndFirstToActPreflop(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	button := mustSeatNo(t, cfg, 1)
	bigBlind := mustSeatNo(t, cfg, 2)
	state, err := StartNewHand(StartNewHandInput{
		TableID:    "table-1",
		HandNo:     1,
		Seats:      mustSeats(t, cfg, 1, 2),
		ButtonSeat: button,
		Config:     cfg,
	})
	if err != nil {
		t.Fatalf("StartNewHand failed: %v", err)
	}

	buttonSeat := findSeat(t, state, button)
	bbSeat := findSeat(t, state, bigBlind)
	if buttonSeat.CommittedInRound != cfg.SmallBlind {
		t.Fatalf("expected button seat %d to post small blind %d, got %d", button, cfg.SmallBlind, buttonSeat.CommittedInRound)
	}
	if bbSeat.CommittedInRound != cfg.BigBlind {
		t.Fatalf("expected seat %d to post big blind %d, got %d", bigBlind, cfg.BigBlind, bbSeat.CommittedInRound)
	}
	if state.ActingSeat != button {
		t.Fatalf("expected button seat %d to act first preflop in heads-up, got %d", button, state.ActingSeat)
	}
}

func TestStartNewHandDealsTwoCardsPerActiveSeatWithNoDuplicates(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	state, err := StartNewHand(StartNewHandInput{
		TableID:    "table-1",
		HandNo:     1,
		Seats:      mustSeats(t, cfg, 1, 2, 3, 4),
		ButtonSeat: mustSeatNo(t, cfg, 1),
		Config:     cfg,
		Shuffler:   rules.NewSeededShuffler(23),
	})
	if err != nil {
		t.Fatalf("StartNewHand failed: %v", err)
	}

	if len(state.Board) != 0 {
		t.Fatalf("expected empty board preflop, got %d", len(state.Board))
	}
	if len(state.HoleCards) != 4 {
		t.Fatalf("expected 4 hole card entries, got %d", len(state.HoleCards))
	}

	seen := map[string]struct{}{}
	for _, seatCards := range state.HoleCards {
		if len(seatCards.Cards) != 2 {
			t.Fatalf("seat %d expected 2 hole cards, got %d", seatCards.SeatNo, len(seatCards.Cards))
		}
		for _, card := range seatCards.Cards {
			key := cardIdentity(card)
			if _, ok := seen[key]; ok {
				t.Fatalf("duplicate dealt card %s", key)
			}
			seen[key] = struct{}{}
		}
	}
}

func TestApplyActionDealsBoardUsingBurnSequence(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	state, err := StartNewHand(StartNewHandInput{
		TableID:    "table-1",
		HandNo:     1,
		Seats:      mustSeats(t, cfg, 1, 2),
		ButtonSeat: mustSeatNo(t, cfg, 1),
		Config:     cfg,
		Shuffler:   noShuffleShuffler{},
	})
	if err != nil {
		t.Fatalf("StartNewHand failed: %v", err)
	}

	call := mustAction(t, domain.ActionCall, nil)
	check := mustAction(t, domain.ActionCheck, nil)

	state, err = ApplyAction(state, call) // closes preflop -> flop dealt
	if err != nil {
		t.Fatalf("preflop call failed: %v", err)
	}
	if state.Street == domain.StreetPreflop {
		state, err = ApplyAction(state, check)
		if err != nil {
			t.Fatalf("preflop check failed: %v", err)
		}
	}
	assertBoard(t, state.Board, []domain.Card{
		mustCard(t, 7, domain.SuitClubs),
		mustCard(t, 8, domain.SuitClubs),
		mustCard(t, 9, domain.SuitClubs),
	})

	state, err = ApplyAction(state, check)
	if err != nil {
		t.Fatalf("flop check 1 failed: %v", err)
	}
	state, err = ApplyAction(state, check) // closes flop -> turn dealt
	if err != nil {
		t.Fatalf("flop check 2 failed: %v", err)
	}
	assertBoard(t, state.Board, []domain.Card{
		mustCard(t, 7, domain.SuitClubs),
		mustCard(t, 8, domain.SuitClubs),
		mustCard(t, 9, domain.SuitClubs),
		mustCard(t, 11, domain.SuitClubs),
	})

	state, err = ApplyAction(state, check)
	if err != nil {
		t.Fatalf("turn check 1 failed: %v", err)
	}
	state, err = ApplyAction(state, check) // closes turn -> river dealt
	if err != nil {
		t.Fatalf("turn check 2 failed: %v", err)
	}
	assertBoard(t, state.Board, []domain.Card{
		mustCard(t, 7, domain.SuitClubs),
		mustCard(t, 8, domain.SuitClubs),
		mustCard(t, 9, domain.SuitClubs),
		mustCard(t, 11, domain.SuitClubs),
		mustCard(t, 13, domain.SuitClubs),
	})
}

func TestStartNewHandHandlesShortStackBlindPosting(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	seats := mustSeats(t, cfg, 1, 2, 3)
	sbSeat := mustSeatNo(t, cfg, 2)
	seats[1].Stack = 25

	state, err := StartNewHand(StartNewHandInput{
		TableID:    "table-1",
		HandNo:     1,
		Seats:      seats,
		ButtonSeat: mustSeatNo(t, cfg, 1),
		Config:     cfg,
	})
	if err != nil {
		t.Fatalf("StartNewHand failed: %v", err)
	}

	sb := findSeat(t, state, sbSeat)
	if sb.CommittedInRound != 25 {
		t.Fatalf("expected short stack sb commit 25, got %d", sb.CommittedInRound)
	}
	if state.Pot != 25+cfg.BigBlind {
		t.Fatalf("expected pot %d, got %d", 25+cfg.BigBlind, state.Pot)
	}
}

func TestStartNewHandRejectsNoActiveSeats(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	seats := mustSeats(t, cfg, 1, 2)
	seats[0].Status = domain.SeatStatusSittingOut
	seats[1].Status = domain.SeatStatusSittingOut

	_, err := StartNewHand(StartNewHandInput{
		TableID:    "table-1",
		HandNo:     1,
		Seats:      seats,
		ButtonSeat: mustSeatNo(t, cfg, 1),
		Config:     cfg,
	})
	if !errors.Is(err, ErrNoActiveSeats) {
		t.Fatalf("expected ErrNoActiveSeats, got %v", err)
	}
}

func TestApplyActionRejectsIllegalCheckWhenFacingBet(t *testing.T) {
	t.Parallel()

	state := startedFourSeatHand(t)
	check, err := domain.NewAction(domain.ActionCheck, nil)
	if err != nil {
		t.Fatalf("NewAction failed: %v", err)
	}

	_, err = ApplyAction(state, check)
	if !errors.Is(err, ErrIllegalAction) {
		t.Fatalf("expected ErrIllegalAction, got %v", err)
	}
}

func TestApplyActionCallFoldProgressionAndImmediateWin(t *testing.T) {
	t.Parallel()

	state := startedFourSeatHand(t)
	call := mustAction(t, domain.ActionCall, nil)
	fold := mustAction(t, domain.ActionFold, nil)

	next, err := ApplyAction(state, call)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if next.ActingSeat != mustSeatNo(t, domain.DefaultV0TableConfig(), 1) {
		t.Fatalf("expected acting seat 1, got %d", next.ActingSeat)
	}

	next, err = ApplyAction(next, fold)
	if err != nil {
		t.Fatalf("first fold failed: %v", err)
	}
	next, err = ApplyAction(next, fold)
	if err != nil {
		t.Fatalf("second fold failed: %v", err)
	}
	next, err = ApplyAction(next, fold)
	if err != nil {
		t.Fatalf("third fold failed: %v", err)
	}

	if next.Phase != domain.HandPhaseComplete {
		t.Fatalf("expected hand phase complete, got %q", next.Phase)
	}
}

func TestApplyActionPreflopClosesToFlop(t *testing.T) {
	t.Parallel()

	state := startedFourSeatHand(t)
	call := mustAction(t, domain.ActionCall, nil)
	check := mustAction(t, domain.ActionCheck, nil)

	var err error
	state, err = ApplyAction(state, call)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	state, err = ApplyAction(state, call)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	state, err = ApplyAction(state, call)
	if err != nil {
		t.Fatalf("third call failed: %v", err)
	}
	state, err = ApplyAction(state, check)
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}

	if state.Street != domain.StreetFlop {
		t.Fatalf("expected street %q, got %q", domain.StreetFlop, state.Street)
	}
	if len(state.Board) != 3 {
		t.Fatalf("expected flop board size 3, got %d", len(state.Board))
	}
}

func TestApplyActionRiverClosureMovesToShowdown(t *testing.T) {
	t.Parallel()

	state := startedTwoSeatPostFlopRound(t, domain.StreetRiver)
	check := mustAction(t, domain.ActionCheck, nil)

	var err error
	state, err = ApplyAction(state, check)
	if err != nil {
		t.Fatalf("first check failed: %v", err)
	}
	state, err = ApplyAction(state, check)
	if err != nil {
		t.Fatalf("second check failed: %v", err)
	}

	if state.Phase != domain.HandPhaseShowdown {
		t.Fatalf("expected phase %q, got %q", domain.HandPhaseShowdown, state.Phase)
	}
}

func TestApplyActionRejectsOnCompletedHand(t *testing.T) {
	t.Parallel()

	state := startedFourSeatHand(t)
	state.Phase = domain.HandPhaseComplete

	_, err := ApplyAction(state, mustAction(t, domain.ActionFold, nil))
	if !errors.Is(err, ErrHandAlreadyComplete) {
		t.Fatalf("expected ErrHandAlreadyComplete, got %v", err)
	}
}

func TestApplyActionPreservesChipConservation(t *testing.T) {
	t.Parallel()

	state := startedFourSeatHand(t)
	initialTotal := chipTotal(state)

	actions := []domain.Action{
		mustAction(t, domain.ActionCall, nil),
		mustAction(t, domain.ActionCall, nil),
		mustAction(t, domain.ActionCall, nil),
		mustAction(t, domain.ActionCheck, nil),
		mustAction(t, domain.ActionCheck, nil),
		mustAction(t, domain.ActionCheck, nil),
		mustAction(t, domain.ActionCheck, nil),
		mustAction(t, domain.ActionCheck, nil),
	}

	var err error
	for _, action := range actions {
		state, err = ApplyAction(state, action)
		if err != nil {
			t.Fatalf("ApplyAction failed: %v", err)
		}
		if chipTotal(state) != initialTotal {
			t.Fatalf("chip conservation violated: want %d got %d", initialTotal, chipTotal(state))
		}
	}
}

func startedFourSeatHand(t *testing.T) domain.HandState {
	t.Helper()

	cfg := domain.DefaultV0TableConfig()
	state, err := StartNewHand(StartNewHandInput{
		TableID:    "table-1",
		HandNo:     1,
		Seats:      mustSeats(t, cfg, 1, 2, 3, 4),
		ButtonSeat: mustSeatNo(t, cfg, 1),
		Config:     cfg,
	})
	if err != nil {
		t.Fatalf("StartNewHand failed: %v", err)
	}

	return state
}

func startedTwoSeatPostFlopRound(t *testing.T, street domain.Street) domain.HandState {
	t.Helper()

	cfg := domain.DefaultV0TableConfig()
	seat1 := mustSeatNo(t, cfg, 1)
	seat2 := mustSeatNo(t, cfg, 2)

	state, err := domain.NewHandState("table-1", 9, seat1, seat1, []domain.SeatState{
		domain.NewSeatState(seat1, cfg.StartingStack),
		domain.NewSeatState(seat2, cfg.StartingStack),
	}, cfg)
	if err != nil {
		t.Fatalf("NewHandState failed: %v", err)
	}

	state.Phase = domain.HandPhaseBetting
	state.Street = street
	state.Board = make([]domain.Card, boardSizeForStreet(street))
	state.ActionOrderStartSeat = seat1
	state.LastAggressorSeat = nil
	state.MinRaiseTo = cfg.BigBlind
	state.CurrentBet = 0

	return state
}

func mustSeats(t *testing.T, cfg domain.TableConfig, seatNumbers ...uint8) []domain.SeatState {
	t.Helper()

	seats := make([]domain.SeatState, 0, len(seatNumbers))
	for _, n := range seatNumbers {
		seats = append(seats, domain.NewSeatState(mustSeatNo(t, cfg, n), cfg.StartingStack))
	}

	return seats
}

func mustSeatNo(t *testing.T, cfg domain.TableConfig, seat uint8) domain.SeatNo {
	t.Helper()

	seatNo, err := domain.NewSeatNo(seat, cfg.MaxSeats)
	if err != nil {
		t.Fatalf("NewSeatNo failed: %v", err)
	}

	return seatNo
}

func findSeat(t *testing.T, state domain.HandState, seatNo domain.SeatNo) domain.SeatState {
	t.Helper()

	for _, seat := range state.Seats {
		if seat.SeatNo == seatNo {
			return seat
		}
	}
	t.Fatalf("seat %d not found", seatNo)
	return domain.SeatState{}
}

func mustAction(t *testing.T, kind domain.ActionKind, amount *uint32) domain.Action {
	t.Helper()

	action, err := domain.NewAction(kind, amount)
	if err != nil {
		t.Fatalf("NewAction failed: %v", err)
	}
	return action
}

func chipTotal(state domain.HandState) uint32 {
	total := state.Pot
	for _, seat := range state.Seats {
		total += seat.Stack
	}
	return total
}

type noShuffleShuffler struct{}

func (noShuffleShuffler) Shuffle(_ []domain.Card) error {
	return nil
}

func cardIdentity(card domain.Card) string {
	return string(card.Suit) + "-" + string(rune(card.Rank))
}

func mustCard(t *testing.T, rank uint8, suit domain.Suit) domain.Card {
	t.Helper()
	r, err := domain.NewRank(rank)
	if err != nil {
		t.Fatalf("NewRank failed: %v", err)
	}
	return domain.NewCard(r, suit)
}

func assertBoard(t *testing.T, board []domain.Card, want []domain.Card) {
	t.Helper()
	if len(board) != len(want) {
		t.Fatalf("expected board length %d, got %d", len(want), len(board))
	}
	for i := range want {
		if board[i] != want[i] {
			t.Fatalf("board[%d] mismatch: want %+v got %+v", i, want[i], board[i])
		}
	}
}

func boardSizeForStreet(street domain.Street) int {
	switch street {
	case domain.StreetPreflop:
		return 0
	case domain.StreetFlop:
		return 3
	case domain.StreetTurn:
		return 4
	case domain.StreetRiver:
		return 5
	default:
		return 0
	}
}
