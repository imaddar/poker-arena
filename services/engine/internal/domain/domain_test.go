package domain

import (
	"errors"
	"testing"
)

func TestNewActionRequiresAmountForRaise(t *testing.T) {
	t.Parallel()

	_, err := NewAction(ActionRaise, nil)
	if err == nil {
		t.Fatal("expected error for raise without amount")
	}
}

func TestNewHandStateRejectsDuplicateSeats(t *testing.T) {
	t.Parallel()

	cfg := DefaultV0TableConfig()
	seatNo, err := NewSeatNo(1, cfg.MaxSeats)
	if err != nil {
		t.Fatalf("NewSeatNo failed: %v", err)
	}

	seats := []SeatState{
		NewSeatState(seatNo, cfg.StartingStack),
		NewSeatState(seatNo, cfg.StartingStack),
	}

	_, err = NewHandState("table-1", 1, seatNo, seatNo, seats, cfg)
	if !errors.Is(err, ErrDuplicateSeat) {
		t.Fatalf("expected ErrDuplicateSeat, got %v", err)
	}
}

func TestTableConfigValidateRejectsZeroBlindAmounts(t *testing.T) {
	t.Parallel()

	t.Run("small blind", func(t *testing.T) {
		t.Parallel()

		cfg := DefaultV0TableConfig()
		cfg.SmallBlind = 0
		if err := cfg.Validate(); !errors.Is(err, ErrInvalidBlindAmount) {
			t.Fatalf("expected ErrInvalidBlindAmount, got %v", err)
		}
	})

	t.Run("big blind", func(t *testing.T) {
		t.Parallel()

		cfg := DefaultV0TableConfig()
		cfg.SmallBlind = 0
		cfg.BigBlind = 0
		if err := cfg.Validate(); !errors.Is(err, ErrInvalidBlindAmount) {
			t.Fatalf("expected ErrInvalidBlindAmount, got %v", err)
		}
	})
}
