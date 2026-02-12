package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	"github.com/imaddar/poker-arena/services/engine/internal/rules"
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
	HandNo          uint64              `json:"hand_no"`
	Phase           domain.HandPhase    `json:"phase"`
	Actions         int                 `json:"actions"`
	Fallbacks       int                 `json:"fallbacks"`
	PotEnd          uint32              `json:"pot_end"`
	Board           []string            `json:"board"`
	ShowdownAwards  []runReportAward    `json:"showdown_awards,omitempty"`
	ShowdownWinners []runReportShowdown `json:"showdown_winners,omitempty"`
	StacksAfter     []runReportSeat     `json:"stacks_after"`
	Timeline        []runReportAction   `json:"timeline"`
}

type runReportAward struct {
	Amount uint32          `json:"amount"`
	Seats  []domain.SeatNo `json:"seats"`
	Reason string          `json:"reason"`
}

type runReportShowdown struct {
	Seat      domain.SeatNo `json:"seat"`
	Won       uint32        `json:"won"`
	HoleCards []string      `json:"hole_cards"`
	BestHand  string        `json:"best_hand"`
	HowWon    string        `json:"how_won"`
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
		timelineByHand[event.HandNo] = append(timelineByHand[event.HandNo], mapActionEvent(event))
	}

	for _, summary := range input.Result.HandSummaries {
		report.Hands = append(report.Hands, buildRunReportHand(summary, timelineByHand[summary.HandNo]))
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
		b.WriteString(renderHandSection(hand, previous))
	}

	b.WriteString(renderRunCompletion(report))

	return b.String()
}

func renderRunCompletion(report runReport) string {
	var b strings.Builder
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

func formatSeatNoList(seats []domain.SeatNo) string {
	parts := make([]string, 0, len(seats))
	for _, seat := range seats {
		parts = append(parts, fmt.Sprintf("seat%d", seat))
	}
	return strings.Join(parts, ",")
}

func mapBoard(board []domain.Card) []string {
	formatted := make([]string, 0, 5)
	for _, card := range board {
		formatted = append(formatted, formatCard(card))
	}
	return formatted
}

func mapAwards(awards []domain.PotAward) []runReportAward {
	mapped := make([]runReportAward, 0, len(awards))
	for _, award := range awards {
		mapped = append(mapped, runReportAward{
			Amount: award.Amount,
			Seats:  append([]domain.SeatNo(nil), award.Seats...),
			Reason: award.Reason,
		})
	}
	return mapped
}

func mapActionEvent(event actionEvent) runReportAction {
	return runReportAction{
		Street: event.Street,
		Seat:   event.Seat,
		Action: event.Action,
		Amount: event.Amount,
	}
}

func buildRunReportHand(summary tablerunner.HandSummary, timeline []runReportAction) runReportHand {
	showdownAwards := mapAwards(summary.FinalState.ShowdownAwards)
	return runReportHand{
		HandNo:          summary.HandNo,
		Phase:           summary.FinalPhase,
		Actions:         summary.ActionCount,
		Fallbacks:       summary.FallbackCount,
		PotEnd:          summary.FinalState.Pot,
		Board:           mapBoard(summary.FinalState.Board),
		ShowdownAwards:  showdownAwards,
		ShowdownWinners: mapShowdownWinners(summary.FinalState, showdownAwards),
		StacksAfter:     mapSeats(summary.FinalState.Seats),
		Timeline:        timeline,
	}
}

func renderHandSection(hand runReportHand, previous map[domain.SeatNo]uint32) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<----- HAND %d ----->\n", hand.HandNo))
	b.WriteString(fmt.Sprintf("phase: %s\n", hand.Phase))
	b.WriteString(fmt.Sprintf("actions: %d\n", hand.Actions))
	b.WriteString(fmt.Sprintf("fallbacks: %d\n", hand.Fallbacks))
	b.WriteString(fmt.Sprintf("pot_end: %d\n", hand.PotEnd))
	b.WriteString(fmt.Sprintf("board: %s\n", strings.Join(hand.Board, " ")))
	b.WriteString("showdown_awards:\n")
	if len(hand.ShowdownAwards) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, award := range hand.ShowdownAwards {
		b.WriteString(fmt.Sprintf("  %s amount=%d seats=%s\n", award.Reason, award.Amount, formatSeatNoList(award.Seats)))
	}
	b.WriteString("showdown_results:\n")
	if len(hand.ShowdownWinners) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, winner := range hand.ShowdownWinners {
		b.WriteString(fmt.Sprintf(
			"  seat%d won %d with %s (hole: %s) via %s\n",
			winner.Seat,
			winner.Won,
			winner.BestHand,
			strings.Join(winner.HoleCards, " "),
			winner.HowWon,
		))
	}
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
	b.WriteString(fmt.Sprintf("<----- END HAND %d ----->\n\n", hand.HandNo))
	return b.String()
}

func mapShowdownWinners(state domain.HandState, awards []runReportAward) []runReportShowdown {
	wonBySeat := map[domain.SeatNo]uint32{}
	reasonsBySeat := map[domain.SeatNo]map[string]struct{}{}
	for _, award := range awards {
		if len(award.Seats) == 0 {
			continue
		}
		share := award.Amount / uint32(len(award.Seats))
		odd := award.Amount % uint32(len(award.Seats))
		for i, seat := range award.Seats {
			wonBySeat[seat] += share
			if uint32(i) < odd {
				wonBySeat[seat]++
			}
			if _, ok := reasonsBySeat[seat]; !ok {
				reasonsBySeat[seat] = map[string]struct{}{}
			}
			reasonsBySeat[seat][award.Reason] = struct{}{}
		}
	}

	holeBySeat := map[domain.SeatNo][]domain.Card{}
	for _, seatCards := range state.HoleCards {
		holeBySeat[seatCards.SeatNo] = append([]domain.Card(nil), seatCards.Cards...)
	}

	winners := make([]runReportShowdown, 0, len(wonBySeat))
	for seat, won := range wonBySeat {
		hole := holeBySeat[seat]
		holeFormatted := make([]string, 0, len(hole))
		for _, card := range hole {
			holeFormatted = append(holeFormatted, formatCard(card))
		}
		if len(holeFormatted) == 0 {
			holeFormatted = []string{"--", "--"}
		}

		bestHand := "unavailable"
		if len(hole) == 2 && len(state.Board) == 5 {
			rank := rules.EvaluateBestHand(hole, state.Board)
			bestHand = handCategoryLabel(rank.Category)
		}

		reasons := make([]string, 0, len(reasonsBySeat[seat]))
		for reason := range reasonsBySeat[seat] {
			reasons = append(reasons, reason)
		}
		sort.Strings(reasons)

		winners = append(winners, runReportShowdown{
			Seat:      seat,
			Won:       won,
			HoleCards: holeFormatted,
			BestHand:  bestHand,
			HowWon:    strings.Join(reasons, "+"),
		})
	}

	sort.Slice(winners, func(i, j int) bool {
		if winners[i].Won == winners[j].Won {
			return winners[i].Seat < winners[j].Seat
		}
		return winners[i].Won > winners[j].Won
	})

	return winners
}

func handCategoryLabel(category rules.HandCategory) string {
	switch category {
	case rules.HandCategoryStraightFlush:
		return "Straight Flush"
	case rules.HandCategoryFourOfAKind:
		return "Four of a Kind"
	case rules.HandCategoryFullHouse:
		return "Full House"
	case rules.HandCategoryFlush:
		return "Flush"
	case rules.HandCategoryStraight:
		return "Straight"
	case rules.HandCategoryThreeOfAKind:
		return "Three of a Kind"
	case rules.HandCategoryTwoPair:
		return "Two Pair"
	case rules.HandCategoryOnePair:
		return "One Pair"
	case rules.HandCategoryHighCard:
		return "High Card"
	default:
		return "Unknown"
	}
}
