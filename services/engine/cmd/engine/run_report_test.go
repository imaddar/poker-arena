package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	"github.com/imaddar/poker-arena/services/engine/internal/tablerunner"
)

func TestRenderRunOutputIncludesSectionsAndTimeline(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	seat1 := mustSeatNo(t, cfg, 1)
	seat2 := mustSeatNo(t, cfg, 2)

	result := tablerunner.RunTableResult{
		HandsCompleted: 1,
		FinalButton:    seat2,
		FinalSeats: []domain.SeatState{
			{SeatNo: seat1, Stack: 9800},
			{SeatNo: seat2, Stack: 10200},
		},
		TotalActions:   8,
		TotalFallbacks: 0,
		HandSummaries: []tablerunner.HandSummary{
			{
				HandNo:        1,
				FinalPhase:    domain.HandPhaseShowdown,
				ActionCount:   8,
				FallbackCount: 0,
				FinalState: domain.HandState{
					Pot: 0,
					Board: []domain.Card{
						mustCard(t, "As"),
						mustCard(t, "Kd"),
						mustCard(t, "Qc"),
						mustCard(t, "Jh"),
						mustCard(t, "Ts"),
					},
					ShowdownAwards: []domain.PotAward{
						{Amount: 400, Seats: []domain.SeatNo{seat2}, Reason: "main_pot"},
					},
					HoleCards: []domain.SeatCards{
						{
							SeatNo: seat2,
							Cards: []domain.Card{
								mustCard(t, "Ah"),
								mustCard(t, "Ad"),
							},
						},
					},
					Seats: []domain.SeatState{
						{SeatNo: seat1, Stack: 9800},
						{SeatNo: seat2, Stack: 10200},
					},
				},
			},
		},
	}

	timeline := []actionEvent{
		{HandNo: 1, Street: domain.StreetPreflop, Seat: seat2, Action: domain.ActionCall},
		{HandNo: 1, Street: domain.StreetPreflop, Seat: seat1, Action: domain.ActionCheck},
	}

	report := buildRunReport(buildRunReportInput{
		Mode:           "play",
		TableID:        "local-table-1",
		HandsRequested: 1,
		HumanSeat:      &seat1,
		InitialSeats: []domain.SeatState{
			{SeatNo: seat1, Stack: cfg.StartingStack},
			{SeatNo: seat2, Stack: cfg.StartingStack},
		},
		Result:   result,
		Timeline: timeline,
	})

	output := renderRunOutput(report)
	checks := []string{
		"=== Poker Arena Local Run ===",
		"<----- HAND 1 ----->",
		"action timeline:",
		"preflop seat2 call",
		"board: As Kd Qc Jh Ts",
		"showdown_awards:",
		"main_pot amount=400 seats=seat2",
		"showdown_results:",
		"seat2 won 400 with Straight (hole: Ah Ad) via main_pot",
		"seat 1: 9800 (-200)",
		"<----- END HAND 1 ----->",
		"=== Run Complete ===",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Fatalf("expected output to contain %q, got:\n%s", check, output)
		}
	}
}

func TestWriteRunReportJSONWritesValidSchema(t *testing.T) {
	t.Parallel()

	report := runReport{
		TableID:        "local-table-1",
		Mode:           "play",
		HandsRequested: 2,
		HandsCompleted: 2,
		TotalActions:   16,
		TotalFallbacks: 0,
		FinalButton:    2,
		FinalSeats: []runReportSeat{
			{SeatNo: 1, Stack: 9800},
			{SeatNo: 2, Stack: 10200},
		},
		Hands: []runReportHand{
			{HandNo: 1, Phase: domain.HandPhaseShowdown, Actions: 8},
			{HandNo: 2, Phase: domain.HandPhaseShowdown, Actions: 8},
		},
	}

	path := filepath.Join(t.TempDir(), "run.json")
	if err := writeRunReportJSON(path, report); err != nil {
		t.Fatalf("writeRunReportJSON failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}

	if parsed["table_id"] != "local-table-1" {
		t.Fatalf("expected table_id local-table-1, got %v", parsed["table_id"])
	}
	if parsed["hands_completed"] != float64(2) {
		t.Fatalf("expected hands_completed 2, got %v", parsed["hands_completed"])
	}

	hands, ok := parsed["hands"].([]any)
	if !ok || len(hands) != 2 {
		t.Fatalf("expected two hands in JSON payload, got %v", parsed["hands"])
	}
}

func mustCard(t *testing.T, value string) domain.Card {
	t.Helper()
	if len(value) != 2 {
		t.Fatalf("invalid card format %q", value)
	}

	var rank uint8
	switch value[0] {
	case 'A':
		rank = 14
	case 'K':
		rank = 13
	case 'Q':
		rank = 12
	case 'J':
		rank = 11
	case 'T':
		rank = 10
	default:
		rank = value[0] - '0'
	}

	r, err := domain.NewRank(rank)
	if err != nil {
		t.Fatalf("invalid rank in %q: %v", value, err)
	}

	var suit domain.Suit
	switch value[1] {
	case 'c':
		suit = domain.SuitClubs
	case 'd':
		suit = domain.SuitDiamonds
	case 'h':
		suit = domain.SuitHearts
	case 's':
		suit = domain.SuitSpades
	default:
		t.Fatalf("invalid suit in %q", value)
	}

	return domain.NewCard(r, suit)
}
