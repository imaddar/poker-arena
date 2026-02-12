package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	"github.com/imaddar/poker-arena/services/engine/internal/tablerunner"
)

func main() {
	mode := flag.String("mode", "sim", "run mode: sim or play")
	hands := flag.Int("hands", 0, "number of hands to run (defaults: sim=100, play=1)")
	humanSeatRaw := flag.Int("human-seat", 1, "human-controlled seat number when mode=play")
	outPath := flag.String("out", "", "optional path to write JSON run report")
	flag.Parse()

	cfg := domain.DefaultV0TableConfig()
	seat1, err := domain.NewSeatNo(1, cfg.MaxSeats)
	if err != nil {
		fmt.Fprintf(os.Stderr, "simulation failed: %v\n", err)
		os.Exit(1)
	}
	seat2, err := domain.NewSeatNo(2, cfg.MaxSeats)
	if err != nil {
		fmt.Fprintf(os.Stderr, "simulation failed: %v\n", err)
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

	events := make([]actionEvent, 0, runHands*8)
	provider := tablerunner.ActionProvider(recordingProvider{
		inner: deterministicProvider{},
		recordAction: func(event actionEvent) {
			events = append(events, event)
		},
	})
	if *mode == "play" {
		humanSeat, err := domain.NewSeatNo(uint8(*humanSeatRaw), cfg.MaxSeats)
		if err != nil {
			fmt.Fprintf(os.Stderr, "simulation failed: %v\n", err)
			os.Exit(1)
		}
		provider = seatProvider{
			humanSeat: humanSeat,
			human:     newHumanProvider(os.Stdin, os.Stdout),
			bot:       deterministicProvider{},
			recordAction: func(event actionEvent) {
				events = append(events, event)
			},
		}

		runWithReport(
			*mode,
			*outPath,
			"local-table-1",
			runHands,
			seats,
			&humanSeat,
			seat1,
			cfg,
			provider,
			&events,
		)
		return
	}

	runWithReport(
		*mode,
		*outPath,
		"local-table-1",
		runHands,
		seats,
		nil,
		seat1,
		cfg,
		provider,
		&events,
	)
}

func runWithReport(
	mode string,
	outPath string,
	tableID string,
	hands int,
	initialSeats []domain.SeatState,
	humanSeat *domain.SeatNo,
	buttonSeat domain.SeatNo,
	cfg domain.TableConfig,
	provider tablerunner.ActionProvider,
	events *[]actionEvent,
) {
	livePrevious := make(map[domain.SeatNo]uint32, len(initialSeats))
	for _, seat := range initialSeats {
		livePrevious[seat.SeatNo] = seat.Stack
	}

	runnerConfig := tablerunner.RunnerConfig{}
	if mode == "play" {
		runnerConfig.OnHandComplete = func(summary tablerunner.HandSummary) {
			timeline := timelineForHand(*events, summary.HandNo)
			hand := buildRunReportHand(summary, timeline)
			fmt.Print(renderHandSection(hand, livePrevious))
		}
	}

	runner := tablerunner.New(provider, runnerConfig)
	result, err := runner.RunTable(context.Background(), tablerunner.RunTableInput{
		TableID:      tableID,
		StartingHand: 1,
		HandsToRun:   hands,
		ButtonSeat:   buttonSeat,
		Seats:        initialSeats,
		Config:       cfg,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "simulation failed: %v\n", err)
		os.Exit(1)
	}

	report := buildRunReport(buildRunReportInput{
		Mode:           mode,
		TableID:        tableID,
		HandsRequested: hands,
		HumanSeat:      humanSeat,
		InitialSeats:   initialSeats,
		Result:         result,
		Timeline:       append([]actionEvent(nil), (*events)...),
	})

	if mode == "play" {
		fmt.Print(renderRunCompletion(report))
	} else {
		fmt.Print(renderRunOutput(report))
	}

	if outPath != "" {
		if err := writeRunReportJSON(outPath, report); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write report json: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("wrote run report: %s\n", outPath)
	}
}

func timelineForHand(events []actionEvent, handNo uint64) []runReportAction {
	timeline := make([]runReportAction, 0, 16)
	for _, event := range events {
		if event.HandNo != handNo {
			continue
		}
		timeline = append(timeline, mapActionEvent(event))
	}
	return timeline
}
