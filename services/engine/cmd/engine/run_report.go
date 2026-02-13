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
	w := 50

	b.WriteString("\n")
	b.WriteString("  ╔" + strings.Repeat("═", w) + "╗\n")
	b.WriteString(fmt.Sprintf("  ║%-*s║\n", w, centerReportText("♠ ♥ ♦ ♣  POKER ARENA  ♣ ♦ ♥ ♠", w)))
	b.WriteString("  ╠" + strings.Repeat("═", w) + "╣\n")
	b.WriteString(fmt.Sprintf("  ║  Mode:    %-*s║\n", w-12, report.Mode))
	b.WriteString(fmt.Sprintf("  ║  Table:   %-*s║\n", w-12, report.TableID))
	if report.HumanSeat != nil {
		b.WriteString(fmt.Sprintf("  ║  Human:   Seat %-*d║\n", w-17, *report.HumanSeat))
	}
	b.WriteString(fmt.Sprintf("  ║  Hands:   %-*d║\n", w-12, report.HandsRequested))
	b.WriteString(fmt.Sprintf("  ║  Stacks:  %-*s║\n", w-12, formatStackList(report.StartingSeats)))
	b.WriteString("  ╚" + strings.Repeat("═", w) + "╝\n\n")

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
	w := 50

	b.WriteString("  ╔" + strings.Repeat("═", w) + "╗\n")
	b.WriteString(fmt.Sprintf("  ║%-*s║\n", w, centerReportText("✓ RUN COMPLETE", w)))
	b.WriteString("  ╠" + strings.Repeat("═", w) + "╣\n")
	b.WriteString(fmt.Sprintf("  ║  Hands Completed:  %-*d║\n", w-22, report.HandsCompleted))
	b.WriteString(fmt.Sprintf("  ║  Total Actions:    %-*d║\n", w-22, report.TotalActions))
	b.WriteString(fmt.Sprintf("  ║  Total Fallbacks:  %-*d║\n", w-22, report.TotalFallbacks))
	b.WriteString(fmt.Sprintf("  ║  Final Button:     Seat %-*d║\n", w-27, report.FinalButton))
	b.WriteString(fmt.Sprintf("  ║  Final Stacks:     %-*s║\n", w-22, formatStackList(report.FinalSeats)))
	b.WriteString("  ╚" + strings.Repeat("═", w) + "╝\n")
	return b.String()
}

func centerReportText(text string, width int) string {
	l := len([]rune(text))
	if l >= width {
		return text
	}
	left := (width - l) / 2
	right := width - l - left
	return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
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
	w := 56

	// Hand header
	b.WriteString(fmt.Sprintf("  ┌%s┐\n", strings.Repeat("─", w)))
	b.WriteString(fmt.Sprintf("  │%-*s│\n", w, centerReportText(fmt.Sprintf("♠ HAND %d ♠", hand.HandNo), w)))
	b.WriteString(fmt.Sprintf("  ├%s┤\n", strings.Repeat("─", w)))

	// Hand info
	board := strings.Join(hand.Board, " ")
	if board == "" {
		board = "(none)"
	}
	b.WriteString(fmt.Sprintf("  │  Phase: %-10s  Actions: %-4d Fallbacks: %-*d│\n", hand.Phase, hand.Actions, w-47, hand.Fallbacks))
	b.WriteString(fmt.Sprintf("  │  Pot: %-12d  Board: %-*s│\n", hand.PotEnd, w-30, board))

	// Awards
	b.WriteString(fmt.Sprintf("  ├%s┤\n", strings.Repeat("─", w)))
	b.WriteString(fmt.Sprintf("  │%-*s│\n", w, "  🏆 Showdown Awards"))
	if len(hand.ShowdownAwards) == 0 {
		b.WriteString(fmt.Sprintf("  │%-*s│\n", w, "    (none)"))
	}
	for _, award := range hand.ShowdownAwards {
		line := fmt.Sprintf("    • %s → %d to %s", award.Reason, award.Amount, formatSeatNoList(award.Seats))
		b.WriteString(fmt.Sprintf("  │%-*s│\n", w, line))
	}

	// Showdown results
	b.WriteString(fmt.Sprintf("  │%-*s│\n", w, "  🏅 Showdown Results"))
	if len(hand.ShowdownWinners) == 0 {
		b.WriteString(fmt.Sprintf("  │%-*s│\n", w, "    (none)"))
	}
	for _, winner := range hand.ShowdownWinners {
		line := fmt.Sprintf(
			"    seat%d won %d with %s (hole: %s) via %s",
			winner.Seat,
			winner.Won,
			winner.BestHand,
			strings.Join(winner.HoleCards, " "),
			winner.HowWon,
		)
		b.WriteString(fmt.Sprintf("  │%-*s│\n", w, line))
	}

	// Stacks
	b.WriteString(fmt.Sprintf("  ├%s┤\n", strings.Repeat("─", w)))
	b.WriteString(fmt.Sprintf("  │%-*s│\n", w, "  💰 Stacks"))
	for _, seat := range hand.StacksAfter {
		delta := int64(seat.Stack) - int64(previous[seat.SeatNo])
		deltaStr := fmt.Sprintf("%+d", delta)
		if delta > 0 {
			deltaStr = "▲" + deltaStr
		} else if delta < 0 {
			deltaStr = "▼" + deltaStr
		} else {
			deltaStr = "• " + deltaStr
		}
		line := fmt.Sprintf("    Seat %d: %d  %s", seat.SeatNo, seat.Stack, deltaStr)
		b.WriteString(fmt.Sprintf("  │%-*s│\n", w, line))
		previous[seat.SeatNo] = seat.Stack
	}

	// Timeline
	b.WriteString(fmt.Sprintf("  ├%s┤\n", strings.Repeat("─", w)))
	b.WriteString(fmt.Sprintf("  │%-*s│\n", w, "  🎬 Action Timeline"))
	if len(hand.Timeline) == 0 {
		b.WriteString(fmt.Sprintf("  │%-*s│\n", w, "    (no actions captured)"))
	}
	for idx, action := range hand.Timeline {
		var line string
		if action.Amount != nil {
			line = fmt.Sprintf("    %d) %s seat%d %s %d", idx+1, action.Street, action.Seat, action.Action, *action.Amount)
		} else {
			line = fmt.Sprintf("    %d) %s seat%d %s", idx+1, action.Street, action.Seat, action.Action)
		}
		b.WriteString(fmt.Sprintf("  │%-*s│\n", w, line))
	}

	b.WriteString(fmt.Sprintf("  └%s┘\n\n", strings.Repeat("─", w)))
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
