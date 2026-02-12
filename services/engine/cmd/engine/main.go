package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	"github.com/imaddar/poker-arena/services/engine/internal/tablerunner"
)

func main() {
	mode := flag.String("mode", "sim", "run mode: sim or play")
	hands := flag.Int("hands", 0, "number of hands to run (defaults: sim=100, play=1)")
	humanSeatRaw := flag.Int("human-seat", 1, "human-controlled seat number when mode=play")
	flag.Parse()

	cfg := domain.DefaultV0TableConfig()
	seat1, err := domain.NewSeatNo(1, cfg.MaxSeats)
	if err != nil {
		slog.Error("simulation failed", "error", err)
		os.Exit(1)
	}
	seat2, err := domain.NewSeatNo(2, cfg.MaxSeats)
	if err != nil {
		slog.Error("simulation failed", "error", err)
		os.Exit(1)
	}

	seats := []domain.SeatState{
		domain.NewSeatState(seat1, cfg.StartingStack),
		domain.NewSeatState(seat2, cfg.StartingStack),
	}

	runHands := *hands
	if runHands <= 0 {
		runHands = 100
		if *mode == "play" {
			runHands = 1
		}
	}

	provider := tablerunner.ActionProvider(deterministicProvider{})
	if *mode == "play" {
		humanSeat, err := domain.NewSeatNo(uint8(*humanSeatRaw), cfg.MaxSeats)
		if err != nil {
			slog.Error("simulation failed", "error", err)
			os.Exit(1)
		}
		provider = seatProvider{
			humanSeat: humanSeat,
			human:     newHumanProvider(os.Stdin, os.Stdout),
			bot:       deterministicProvider{},
		}
	}

	runner := tablerunner.New(provider, tablerunner.RunnerConfig{})

	slog.Info("starting local simulation", "mode", *mode, "hands_to_run", runHands, "table_id", "local-table-1")

	result, err := runner.RunTable(context.Background(), tablerunner.RunTableInput{
		TableID:      "local-table-1",
		StartingHand: 1,
		HandsToRun:   runHands,
		ButtonSeat:   seat1,
		Seats:        seats,
		Config:       cfg,
	})
	if err != nil {
		slog.Error("simulation failed", "error", err)
		os.Exit(1)
	}

	for _, hand := range result.HandSummaries {
		slog.Info(
			"hand complete",
			"hand_no", hand.HandNo,
			"phase", hand.FinalPhase,
			"actions", hand.ActionCount,
			"fallbacks", hand.FallbackCount,
		)
	}

	slog.Info(
		"simulation complete",
		"hands_completed", result.HandsCompleted,
		"total_actions", result.TotalActions,
		"total_fallbacks", result.TotalFallbacks,
		"final_button", result.FinalButton,
	)
}
