package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	"github.com/imaddar/poker-arena/services/engine/internal/tablerunner"
)

type actionEvent struct {
	HandNo uint64
	Street domain.Street
	Seat   domain.SeatNo
	Action domain.ActionKind
	Amount *uint32
}

type buildRunReportInput struct {
	Mode           string
	TableID        string
	HandsRequested int
	HumanSeat      *domain.SeatNo
	InitialSeats   []domain.SeatState
	Result         tablerunner.RunTableResult
	Timeline       []actionEvent
}

type runReport struct {
	TableID        string          `json:"table_id"`
	Mode           string          `json:"mode"`
	HandsRequested int             `json:"hands_requested"`
	HandsCompleted int             `json:"hands_completed"`
	TotalActions   int             `json:"total_actions"`
	TotalFallbacks int             `json:"total_fallbacks"`
	FinalButton    domain.SeatNo   `json:"final_button"`
	StartingSeats  []runReportSeat `json:"starting_seats,omitempty"`
	FinalSeats     []runReportSeat `json:"final_seats"`
	Hands          []runReportHand `json:"hands"`
	HumanSeat      *domain.SeatNo  `json:"human_seat,omitempty"`
}

type runReportSeat struct {
	SeatNo domain.SeatNo `json:"seat_no"`
	Stack  uint32        `json:"stack"`
}

type runReportAction struct {
	Street domain.Street     `json:"street"`
	Seat   domain.SeatNo     `json:"seat"`
	Action domain.ActionKind `json:"action"`
	Amount *uint32           `json:"amount,omitempty"`
}

type runReportHand struct {
	HandNo      uint64            `json:"hand_no"`
	Phase       domain.HandPhase  `json:"phase"`
	Actions     int               `json:"actions"`
	Fallbacks   int               `json:"fallbacks"`
	PotEnd      uint32            `json:"pot_end"`
	StacksAfter []runReportSeat   `json:"stacks_after"`
	Timeline    []runReportAction `json:"timeline"`
}

func buildRunReport(input buildRunReportInput) runReport {
	report := runReport{
		TableID:        input.TableID,
		Mode:           input.Mode,
		HandsRequested: input.HandsRequested,
		HandsCompleted: input.Result.HandsCompleted,
		TotalActions:   input.Result.TotalActions,
		TotalFallbacks: input.Result.TotalFallbacks,
		FinalButton:    input.Result.FinalButton,
		StartingSeats:  mapSeats(input.InitialSeats),
		FinalSeats:     mapSeats(input.Result.FinalSeats),
		Hands:          make([]runReportHand, 0, len(input.Result.HandSummaries)),
		HumanSeat:      input.HumanSeat,
	}

	timelineByHand := make(map[uint64][]runReportAction)
	for _, event := range input.Timeline {
		timelineByHand[event.HandNo] = append(timelineByHand[event.HandNo], runReportAction{
			Street: event.Street,
			Seat:   event.Seat,
			Action: event.Action,
			Amount: event.Amount,
		})
	}

	for _, summary := range input.Result.HandSummaries {
		report.Hands = append(report.Hands, runReportHand{
			HandNo:      summary.HandNo,
			Phase:       summary.FinalPhase,
			Actions:     summary.ActionCount,
			Fallbacks:   summary.FallbackCount,
			PotEnd:      summary.FinalState.Pot,
			StacksAfter: mapSeats(summary.FinalState.Seats),
			Timeline:    timelineByHand[summary.HandNo],
		})
	}

	return report
}

func renderRunOutput(report runReport) string {
	var b strings.Builder

	b.WriteString("=== Poker Arena Local Run ===\n")
	b.WriteString(fmt.Sprintf("mode: %s\n", report.Mode))
	b.WriteString(fmt.Sprintf("table: %s\n", report.TableID))
	if report.HumanSeat != nil {
		b.WriteString(fmt.Sprintf("human seat: %d\n", *report.HumanSeat))
	}
	b.WriteString(fmt.Sprintf("hands: %d\n", report.HandsRequested))
	b.WriteString(fmt.Sprintf("starting stacks: %s\n", formatStackList(report.StartingSeats)))
	b.WriteString("============================\n\n")

	previous := make(map[domain.SeatNo]uint32)
	for _, seat := range report.StartingSeats {
		previous[seat.SeatNo] = seat.Stack
	}

	for _, hand := range report.Hands {
		b.WriteString(fmt.Sprintf("--- Hand %d Complete ---\n", hand.HandNo))
		b.WriteString(fmt.Sprintf("phase: %s\n", hand.Phase))
		b.WriteString(fmt.Sprintf("actions: %d\n", hand.Actions))
		b.WriteString(fmt.Sprintf("fallbacks: %d\n", hand.Fallbacks))
		b.WriteString(fmt.Sprintf("pot_end: %d\n", hand.PotEnd))
		b.WriteString("stacks:\n")
		for _, seat := range hand.StacksAfter {
			delta := int64(seat.Stack) - int64(previous[seat.SeatNo])
			b.WriteString(fmt.Sprintf("  seat %d: %d (%+d)\n", seat.SeatNo, seat.Stack, delta))
			previous[seat.SeatNo] = seat.Stack
		}
		b.WriteString("action timeline:\n")
		for idx, action := range hand.Timeline {
			b.WriteString(fmt.Sprintf("  %d) %s seat%d %s", idx+1, action.Street, action.Seat, action.Action))
			if action.Amount != nil {
				b.WriteString(fmt.Sprintf(" %d", *action.Amount))
			}
			b.WriteString("\n")
		}
		if len(hand.Timeline) == 0 {
			b.WriteString("  (no actions captured)\n")
		}
		b.WriteString("-----------------------\n\n")
	}

	b.WriteString("=== Run Complete ===\n")
	b.WriteString(fmt.Sprintf("hands_completed: %d\n", report.HandsCompleted))
	b.WriteString(fmt.Sprintf("total_actions: %d\n", report.TotalActions))
	b.WriteString(fmt.Sprintf("total_fallbacks: %d\n", report.TotalFallbacks))
	b.WriteString(fmt.Sprintf("final_button: %d\n", report.FinalButton))
	b.WriteString(fmt.Sprintf("final_stacks: %s\n", formatStackList(report.FinalSeats)))
	b.WriteString("====================\n")

	return b.String()
}

func writeRunReportJSON(path string, report runReport) error {
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func mapSeats(seats []domain.SeatState) []runReportSeat {
	mapped := make([]runReportSeat, 0, len(seats))
	for _, seat := range seats {
		mapped = append(mapped, runReportSeat{SeatNo: seat.SeatNo, Stack: seat.Stack})
	}
	sort.Slice(mapped, func(i, j int) bool { return mapped[i].SeatNo < mapped[j].SeatNo })
	return mapped
}

func formatStackList(seats []runReportSeat) string {
	parts := make([]string, 0, len(seats))
	for _, seat := range seats {
		parts = append(parts, fmt.Sprintf("seat%d=%d", seat.SeatNo, seat.Stack))
	}
	return strings.Join(parts, " ")
}
