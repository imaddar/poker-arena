package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	"github.com/imaddar/poker-arena/services/engine/internal/tablerunner"
)

var errUnsupportedAction = errors.New("unsupported action")

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

type humanProvider struct {
	in  *bufio.Scanner
	out io.Writer
}

func newHumanProvider(in io.Reader, out io.Writer) humanProvider {
	return humanProvider{in: bufio.NewScanner(in), out: out}
}

func (p humanProvider) NextAction(ctx context.Context, state domain.HandState) (domain.Action, error) {
	for {
		if err := ctx.Err(); err != nil {
			return domain.Action{}, err
		}

		toCall := uint32(0)
		for i := range state.Seats {
			if state.Seats[i].SeatNo == state.ActingSeat {
				if state.CurrentBet > state.Seats[i].CommittedInRound {
					toCall = state.CurrentBet - state.Seats[i].CommittedInRound
				}
				break
			}
		}

		options := "fold(f)/check(k)/bet(b) <amt>"
		if toCall > 0 {
			options = "fold(f)/check(k)/call(c)/raise(r) <amt>"
		}

		fmt.Fprint(p.out, renderMiniPokerTable(state, toCall, options))
		if !p.in.Scan() {
			if err := p.in.Err(); err != nil {
				return domain.Action{}, err
			}
			return domain.Action{}, io.EOF
		}

		rawInput := strings.ToLower(strings.TrimSpace(p.in.Text()))
		if rawInput == "bet" || rawInput == "b" {
			amount := state.MinRaiseTo
			label := "minimum bet"
			kind := domain.ActionBet
			if toCall > 0 {
				label = "minimum raise"
				kind = domain.ActionRaise
			}
			fmt.Fprintf(p.out, "interpreting bare 'bet' as %s to %d\n", label, amount)
			action, err := domain.NewAction(kind, &amount)
			if err != nil {
				fmt.Fprintf(p.out, "invalid action. valid: %s\n", options)
				continue
			}
			if err := validateHumanAction(state, action); err != nil {
				fmt.Fprintf(p.out, "illegal action: %v\n", err)
				continue
			}
			return action, nil
		}

		action, err := parseHumanAction(rawInput)
		if err != nil {
			fmt.Fprintf(p.out, "invalid action. valid: %s\n", options)
			continue
		}
		if err := validateHumanAction(state, action); err != nil {
			fmt.Fprintf(p.out, "illegal action: %v\n", err)
			continue
		}
		if action.Kind == domain.ActionCheck {
			fmt.Fprintf(p.out, "checked on %s\n", state.Street)
		}
		return action, nil
	}
}

func parseHumanAction(input string) (domain.Action, error) {
	normalized := strings.ToLower(strings.TrimSpace(input))
	parts := strings.Fields(normalized)
	if len(parts) == 0 {
		return domain.Action{}, fmt.Errorf("%w: empty action", errUnsupportedAction)
	}

	switch parts[0] {
	case "fold", "f":
		if len(parts) != 1 {
			return domain.Action{}, fmt.Errorf("%w: fold does not take an amount", errUnsupportedAction)
		}
		return domain.NewAction(domain.ActionFold, nil)
	case "check", "k":
		if len(parts) != 1 {
			return domain.Action{}, fmt.Errorf("%w: check does not take an amount", errUnsupportedAction)
		}
		return domain.NewAction(domain.ActionCheck, nil)
	case "call", "c":
		if len(parts) != 1 {
			return domain.Action{}, fmt.Errorf("%w: call does not take an amount", errUnsupportedAction)
		}
		return domain.NewAction(domain.ActionCall, nil)
	case "bet", "b", "raise", "r":
		if len(parts) != 2 {
			return domain.Action{}, fmt.Errorf("%w: %s requires an amount", errUnsupportedAction, parts[0])
		}
		parsed, err := strconv.ParseUint(parts[1], 10, 32)
		if err != nil || parsed == 0 {
			return domain.Action{}, fmt.Errorf("%w: invalid amount %q", errUnsupportedAction, parts[1])
		}
		amount := uint32(parsed)
		if parts[0] == "bet" || parts[0] == "b" {
			return domain.NewAction(domain.ActionBet, &amount)
		}
		return domain.NewAction(domain.ActionRaise, &amount)
	default:
		return domain.Action{}, fmt.Errorf("%w: %q", errUnsupportedAction, input)
	}
}

func validateHumanAction(state domain.HandState, action domain.Action) error {
	actingIdx := -1
	for i := range state.Seats {
		if state.Seats[i].SeatNo == state.ActingSeat {
			actingIdx = i
			break
		}
	}
	if actingIdx < 0 {
		return fmt.Errorf("acting seat %d not found", state.ActingSeat)
	}

	acting := state.Seats[actingIdx]
	toCall := uint32(0)
	if state.CurrentBet > acting.CommittedInRound {
		toCall = state.CurrentBet - acting.CommittedInRound
	}

	switch action.Kind {
	case domain.ActionFold:
		return nil
	case domain.ActionCheck:
		if toCall != 0 {
			return fmt.Errorf("cannot check when to_call is %d", toCall)
		}
		return nil
	case domain.ActionCall:
		if toCall == 0 {
			return errors.New("cannot call when there is no bet to call")
		}
		return nil
	case domain.ActionBet:
		if toCall != 0 || state.CurrentBet != 0 {
			return errors.New("cannot bet when facing an existing bet; use raise")
		}
		if action.Amount == nil || *action.Amount == 0 {
			return errors.New("bet requires a positive amount")
		}
		if *action.Amount > acting.Stack {
			return fmt.Errorf("bet amount %d exceeds stack %d", *action.Amount, acting.Stack)
		}
		return nil
	case domain.ActionRaise:
		if toCall == 0 || state.CurrentBet == 0 {
			return errors.New("cannot raise when there is no existing bet; use bet")
		}
		if action.Amount == nil || *action.Amount == 0 {
			return errors.New("raise requires a positive amount")
		}
		if *action.Amount < state.MinRaiseTo {
			return fmt.Errorf("raise amount %d is below min_raise_to %d", *action.Amount, state.MinRaiseTo)
		}
		if *action.Amount <= acting.CommittedInRound {
			return fmt.Errorf("raise amount %d must exceed your current committed amount %d", *action.Amount, acting.CommittedInRound)
		}
		requiredDelta := *action.Amount - acting.CommittedInRound
		if requiredDelta > acting.Stack {
			return fmt.Errorf("raise requires %d chips but stack is %d", requiredDelta, acting.Stack)
		}
		return nil
	default:
		return fmt.Errorf("unsupported action kind %q", action.Kind)
	}
}

type seatProvider struct {
	humanSeat domain.SeatNo
	human     tablerunner.ActionProvider
	bot       tablerunner.ActionProvider
	out       io.Writer
}

func (p seatProvider) NextAction(ctx context.Context, state domain.HandState) (domain.Action, error) {
	out := p.out
	if out == nil {
		out = os.Stdout
	}
	if state.ActingSeat == p.humanSeat {
		action, err := p.human.NextAction(ctx, state)
		if err != nil {
			return action, err
		}
		fmt.Fprintf(out, "you (seat %d) -> %s\n", state.ActingSeat, formatAction(action))
		return action, nil
	}
	action, err := p.bot.NextAction(ctx, state)
	if err != nil {
		return action, err
	}
	fmt.Fprintf(out, "bot (seat %d) -> %s\n", state.ActingSeat, formatAction(action))
	return action, nil
}

func formatAction(action domain.Action) string {
	if action.Amount == nil {
		return string(action.Kind)
	}
	return fmt.Sprintf("%s %d", action.Kind, *action.Amount)
}

const tablePromptWidth = 58

func renderMiniPokerTable(state domain.HandState, toCall uint32, options string) string {
	lines := []string{
		"MINI ASCII POKER TABLE",
		fmt.Sprintf("Hand #%d | Table: %s", state.HandNo, state.TableID),
		fmt.Sprintf("Street: %s | Pot: %d | To Call: %d", state.Street, state.Pot, toCall),
		fmt.Sprintf("Current Bet: %d | Min Raise To: %d", state.CurrentBet, state.MinRaiseTo),
		fmt.Sprintf("Board: %s", formatBoardCards(state.Board)),
		"                    .----------------------.",
		"                   /                        \\",
		"                  |        TABLE VIEW        |",
		"                   \\                        /",
		"                    '----------------------'",
	}

	for i := range state.Seats {
		lines = append(lines, formatSeatPromptLine(state.Seats[i], state))
	}

	lines = append(lines, fmt.Sprintf("Options: %s", options))

	var builder strings.Builder
	builder.WriteString("+" + strings.Repeat("-", tablePromptWidth+2) + "+\n")
	for _, line := range lines {
		builder.WriteString(framePromptLine(line))
	}
	builder.WriteString("+" + strings.Repeat("-", tablePromptWidth+2) + "+\n")
	builder.WriteString("Action > ")
	return builder.String()
}

func framePromptLine(content string) string {
	if len(content) > tablePromptWidth {
		content = content[:tablePromptWidth]
	}
	return fmt.Sprintf("| %-*s |\n", tablePromptWidth, content)
}

func formatSeatPromptLine(seat domain.SeatState, state domain.HandState) string {
	marker := " "
	if seat.SeatNo == state.ActingSeat {
		marker = ">"
	}

	role := "-"
	switch {
	case seat.SeatNo == state.ActingSeat && seat.SeatNo == state.ButtonSeat:
		role = "A/D"
	case seat.SeatNo == state.ActingSeat:
		role = "A"
	case seat.SeatNo == state.ButtonSeat:
		role = "D"
	}

	extras := make([]string, 0, 2)
	if seat.Folded {
		extras = append(extras, "folded")
	}
	if seat.Status != "" && seat.Status != domain.SeatStatusActive {
		extras = append(extras, string(seat.Status))
	}

	status := ""
	if len(extras) > 0 {
		status = " [" + strings.Join(extras, ", ") + "]"
	}

	return fmt.Sprintf(
		"%s %s Seat %d | stack:%d | in:%d%s",
		marker,
		role,
		seat.SeatNo,
		seat.Stack,
		seat.CommittedInRound,
		status,
	)
}

func formatBoardCards(board []domain.Card) string {
	formatted := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		if i < len(board) {
			formatted = append(formatted, formatCard(board[i]))
			continue
		}
		formatted = append(formatted, "--")
	}
	return strings.Join(formatted, " ")
}

func formatCard(card domain.Card) string {
	return formatRank(card.Rank) + formatSuit(card.Suit)
}

func formatRank(rank domain.Rank) string {
	switch uint8(rank) {
	case 14:
		return "A"
	case 13:
		return "K"
	case 12:
		return "Q"
	case 11:
		return "J"
	case 10:
		return "T"
	default:
		return strconv.FormatUint(uint64(rank), 10)
	}
}

func formatSuit(suit domain.Suit) string {
	switch suit {
	case domain.SuitClubs:
		return "c"
	case domain.SuitDiamonds:
		return "d"
	case domain.SuitHearts:
		return "h"
	case domain.SuitSpades:
		return "s"
	default:
		return "?"
	}
}
