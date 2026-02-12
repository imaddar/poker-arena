package rules

import (
	"testing"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

func TestResolvePots_SingleWinnerTakesAll(t *testing.T) {
	t.Parallel()

	state := showdownState(t, []domain.SeatState{
		seatWithCommit(t, 1, 900, 100, false),
		seatWithCommit(t, 2, 900, 100, false),
	}, []domain.SeatCards{
		{SeatNo: mustSeatNo(t, 1), Cards: cards(t, "As", "Ah")},
		{SeatNo: mustSeatNo(t, 2), Cards: cards(t, "Kd", "Kh")},
	}, cards(t, "2c", "3d", "7h", "9s", "Jc"), mustSeatNo(t, 1), 200)

	resolved, awards, err := ResolvePots(state)
	if err != nil {
		t.Fatalf("ResolvePots failed: %v", err)
	}

	if resolved.Pot != 0 {
		t.Fatalf("expected pot 0, got %d", resolved.Pot)
	}
	if len(awards) != 1 || awards[0].Amount != 200 {
		t.Fatalf("expected single award of 200, got %+v", awards)
	}
	if resolved.Seats[0].Stack != 1100 {
		t.Fatalf("expected seat1 stack 1100, got %d", resolved.Seats[0].Stack)
	}
}

func TestResolvePots_SplitPotOddChipLeftOfButton(t *testing.T) {
	t.Parallel()

	state := showdownState(t, []domain.SeatState{
		seatWithCommit(t, 1, 900, 101, false),
		seatWithCommit(t, 2, 900, 101, false),
		seatWithCommit(t, 3, 900, 101, false),
	}, []domain.SeatCards{
		{SeatNo: mustSeatNo(t, 1), Cards: cards(t, "2c", "3d")},
		{SeatNo: mustSeatNo(t, 2), Cards: cards(t, "2d", "4h")},
		{SeatNo: mustSeatNo(t, 3), Cards: cards(t, "9c", "8d")},
	}, cards(t, "As", "Kd", "Qs", "7c", "2s"), mustSeatNo(t, 1), 303)

	resolved, _, err := ResolvePots(state)
	if err != nil {
		t.Fatalf("ResolvePots failed: %v", err)
	}

	if resolved.Seats[1].Stack != 1052 {
		t.Fatalf("expected odd chip to seat2 (left of button), got %d", resolved.Seats[1].Stack)
	}
	if resolved.Seats[0].Stack != 1051 {
		t.Fatalf("expected seat1 stack 1051, got %d", resolved.Seats[0].Stack)
	}
	if resolved.Seats[2].Stack != 900 {
		t.Fatalf("expected seat3 stack unchanged at 900, got %d", resolved.Seats[2].Stack)
	}
}

func TestResolvePots_OneSidePot(t *testing.T) {
	t.Parallel()

	state := showdownState(t, []domain.SeatState{
		seatWithCommit(t, 1, 700, 300, false),
		seatWithCommit(t, 2, 800, 200, false),
		seatWithCommit(t, 3, 800, 200, false),
	}, []domain.SeatCards{
		{SeatNo: mustSeatNo(t, 1), Cards: cards(t, "As", "Ah")},
		{SeatNo: mustSeatNo(t, 2), Cards: cards(t, "Kd", "Kh")},
		{SeatNo: mustSeatNo(t, 3), Cards: cards(t, "Qd", "Qh")},
	}, cards(t, "2c", "3d", "7h", "9s", "Jc"), mustSeatNo(t, 3), 700)

	resolved, awards, err := ResolvePots(state)
	if err != nil {
		t.Fatalf("ResolvePots failed: %v", err)
	}

	if len(awards) != 2 {
		t.Fatalf("expected main+side awards, got %+v", awards)
	}
	if resolved.Seats[0].Stack != 1400 {
		t.Fatalf("expected seat1 to win both pots, got %d", resolved.Seats[0].Stack)
	}
}

func TestResolvePots_MultipleSidePotsAndFoldedExclusion(t *testing.T) {
	t.Parallel()

	state := showdownState(t, []domain.SeatState{
		seatWithCommit(t, 1, 600, 400, false),
		seatWithCommit(t, 2, 700, 300, false),
		seatWithCommit(t, 3, 800, 200, false),
		seatWithCommit(t, 4, 900, 100, true),
	}, []domain.SeatCards{
		{SeatNo: mustSeatNo(t, 1), Cards: cards(t, "As", "Ah")},
		{SeatNo: mustSeatNo(t, 2), Cards: cards(t, "Kd", "Kh")},
		{SeatNo: mustSeatNo(t, 3), Cards: cards(t, "Qd", "Qh")},
		{SeatNo: mustSeatNo(t, 4), Cards: cards(t, "2d", "2h")},
	}, cards(t, "2c", "3d", "7h", "9s", "Jc"), mustSeatNo(t, 2), 1000)

	resolved, awards, err := ResolvePots(state)
	if err != nil {
		t.Fatalf("ResolvePots failed: %v", err)
	}

	if len(awards) != 4 {
		t.Fatalf("expected 4 awards (main+3 sides), got %d", len(awards))
	}
	if resolved.Seats[0].Stack != 1600 {
		t.Fatalf("expected strongest live hand to collect all eligible pots, got %d", resolved.Seats[0].Stack)
	}
	if resolved.Seats[3].Stack != 900 {
		t.Fatalf("folded seat should not win any pot, got %d", resolved.Seats[3].Stack)
	}
}

func showdownState(t *testing.T, seats []domain.SeatState, hole []domain.SeatCards, board []domain.Card, button domain.SeatNo, pot uint32) domain.HandState {
	t.Helper()
	return domain.HandState{
		ButtonSeat: button,
		Seats:      seats,
		HoleCards:  hole,
		Board:      board,
		Pot:        pot,
		Phase:      domain.HandPhaseShowdown,
	}
}

func seatWithCommit(t *testing.T, seat uint8, stack uint32, committed uint32, folded bool) domain.SeatState {
	t.Helper()
	return domain.SeatState{
		SeatNo:         mustSeatNo(t, seat),
		Stack:          stack,
		TotalCommitted: committed,
		Folded:         folded,
		Status:         domain.SeatStatusActive,
	}
}

func mustSeatNo(t *testing.T, seat uint8) domain.SeatNo {
	t.Helper()
	s, err := domain.NewSeatNo(seat, domain.DefaultMaxSeats)
	if err != nil {
		t.Fatalf("NewSeatNo failed: %v", err)
	}
	return s
}
