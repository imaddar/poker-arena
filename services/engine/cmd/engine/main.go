package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	"github.com/imaddar/poker-arena/services/engine/internal/tablerunner"
)

func main() {
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

	provider := deterministicProvider{}
	runner := tablerunner.New(provider, tablerunner.RunnerConfig{})

	slog.Info("starting local simulation", "hands_to_run", 100, "table_id", "local-table-1")

	result, err := runner.RunTable(context.Background(), tablerunner.RunTableInput{
		TableID:      "local-table-1",
		StartingHand: 1,
		HandsToRun:   100,
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

type deterministicProvider struct{}

func (p deterministicProvider) NextAction(_ context.Context, state domain.HandState) (domain.Action, error) {
	var actingSeat *domain.SeatState
	for i := range state.Seats {
		if state.Seats[i].SeatNo == state.ActingSeat {
			actingSeat = &state.Seats[i]
			break
		}
	}

	if actingSeat == nil {
		return domain.Action{}, tablerunner.ErrRunnerMisconfigured
	}

	if state.CurrentBet > actingSeat.CommittedInRound {
		return domain.NewAction(domain.ActionCall, nil)
	}

	return domain.NewAction(domain.ActionCheck, nil)
}
