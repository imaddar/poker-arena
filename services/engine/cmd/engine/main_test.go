package main

import (
	"testing"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

func TestBuildInitialSeatsCreatesRequestedPlayerCount(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	seats, err := buildInitialSeats(cfg, 6)
	if err != nil {
		t.Fatalf("buildInitialSeats failed: %v", err)
	}
	if len(seats) != 6 {
		t.Fatalf("expected 6 seats, got %d", len(seats))
	}

	for i, seat := range seats {
		want := domain.SeatNo(i + 1)
		if seat.SeatNo != want {
			t.Fatalf("seat index %d: expected seat %d, got %d", i, want, seat.SeatNo)
		}
		if seat.Stack != cfg.StartingStack {
			t.Fatalf("seat %d: expected stack %d, got %d", seat.SeatNo, cfg.StartingStack, seat.Stack)
		}
	}
}

func TestBuildInitialSeatsRejectsOutOfRangePlayerCounts(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()

	if _, err := buildInitialSeats(cfg, 1); err == nil {
		t.Fatal("expected error for players=1")
	}
	if _, err := buildInitialSeats(cfg, 7); err == nil {
		t.Fatal("expected error for players=7")
	}
}
