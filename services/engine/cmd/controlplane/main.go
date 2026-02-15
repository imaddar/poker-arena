package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/imaddar/poker-arena/services/engine/internal/api"
	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	"github.com/imaddar/poker-arena/services/engine/internal/persistence"
	"github.com/imaddar/poker-arena/services/engine/internal/tablerunner"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	repo := persistence.NewInMemoryRepository()
	server := api.NewServer(
		repo,
		func(provider tablerunner.ActionProvider, cfg tablerunner.RunnerConfig) api.Runner {
			return tablerunner.New(provider, cfg)
		},
		func(_ string) (tablerunner.ActionProvider, error) {
			return deterministicProvider{}, nil
		},
	)

	fmt.Fprintf(os.Stdout, "engine control-plane listening on %s\n", *addr)
	if err := http.ListenAndServe(*addr, server); err != nil {
		fmt.Fprintf(os.Stderr, "server failed: %v\n", err)
		os.Exit(1)
	}
}

type deterministicProvider struct{}

func (deterministicProvider) NextAction(_ context.Context, state domain.HandState) (domain.Action, error) {
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

