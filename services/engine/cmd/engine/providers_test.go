package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

func TestParseHumanAction_Check(t *testing.T) {
	t.Parallel()

	action, err := parseHumanAction("check")
	if err != nil {
		t.Fatalf("parseHumanAction failed: %v", err)
	}
	if action.Kind != domain.ActionCheck {
		t.Fatalf("expected check, got %q", action.Kind)
	}
}

func TestParseHumanAction_Call(t *testing.T) {
	t.Parallel()

	action, err := parseHumanAction(" call ")
	if err != nil {
		t.Fatalf("parseHumanAction failed: %v", err)
	}
	if action.Kind != domain.ActionCall {
		t.Fatalf("expected call, got %q", action.Kind)
	}
}

func TestParseHumanAction_Fold(t *testing.T) {
	t.Parallel()

	action, err := parseHumanAction("FOLD")
	if err != nil {
		t.Fatalf("parseHumanAction failed: %v", err)
	}
	if action.Kind != domain.ActionFold {
		t.Fatalf("expected fold, got %q", action.Kind)
	}
}

func TestParseHumanAction_FoldShort(t *testing.T) {
	t.Parallel()

	action, err := parseHumanAction("f")
	if err != nil {
		t.Fatalf("parseHumanAction failed: %v", err)
	}
	if action.Kind != domain.ActionFold {
		t.Fatalf("expected fold, got %q", action.Kind)
	}
}

func TestParseHumanAction_Invalid(t *testing.T) {
	t.Parallel()

	_, err := parseHumanAction("dance")
	if !errors.Is(err, errUnsupportedAction) {
		t.Fatalf("expected errUnsupportedAction, got %v", err)
	}
}

func TestParseHumanAction_BetWithAmount(t *testing.T) {
	t.Parallel()

	action, err := parseHumanAction("bet 200")
	if err != nil {
		t.Fatalf("parseHumanAction failed: %v", err)
	}
	if action.Kind != domain.ActionBet {
		t.Fatalf("expected bet, got %q", action.Kind)
	}
	if action.Amount == nil || *action.Amount != 200 {
		t.Fatalf("expected amount 200, got %v", action.Amount)
	}
}

func TestParseHumanAction_BetShortWithAmount(t *testing.T) {
	t.Parallel()

	action, err := parseHumanAction("b 200")
	if err != nil {
		t.Fatalf("parseHumanAction failed: %v", err)
	}
	if action.Kind != domain.ActionBet {
		t.Fatalf("expected bet, got %q", action.Kind)
	}
	if action.Amount == nil || *action.Amount != 200 {
		t.Fatalf("expected amount 200, got %v", action.Amount)
	}
}

func TestParseHumanAction_RaiseWithAmount(t *testing.T) {
	t.Parallel()

	action, err := parseHumanAction("raise 350")
	if err != nil {
		t.Fatalf("parseHumanAction failed: %v", err)
	}
	if action.Kind != domain.ActionRaise {
		t.Fatalf("expected raise, got %q", action.Kind)
	}
	if action.Amount == nil || *action.Amount != 350 {
		t.Fatalf("expected amount 350, got %v", action.Amount)
	}
}

func TestParseHumanAction_RaiseShortWithAmount(t *testing.T) {
	t.Parallel()

	action, err := parseHumanAction("r 350")
	if err != nil {
		t.Fatalf("parseHumanAction failed: %v", err)
	}
	if action.Kind != domain.ActionRaise {
		t.Fatalf("expected raise, got %q", action.Kind)
	}
	if action.Amount == nil || *action.Amount != 350 {
		t.Fatalf("expected amount 350, got %v", action.Amount)
	}
}

func TestParseHumanAction_BetMissingAmount(t *testing.T) {
	t.Parallel()

	_, err := parseHumanAction("bet")
	if !errors.Is(err, errUnsupportedAction) {
		t.Fatalf("expected errUnsupportedAction, got %v", err)
	}
}

func TestHumanProviderReadsUntilValidAction(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	seat := mustSeatNo(t, cfg, 1)

	in := strings.NewReader("abc\ncheck\n")
	out := &strings.Builder{}
	provider := newHumanProvider(in, out)

	action, err := provider.NextAction(context.Background(), domain.HandState{
		ActingSeat: seat,
		Street:     domain.StreetFlop,
		Seats:      []domain.SeatState{{SeatNo: seat, Stack: 1000, CommittedInRound: 0}},
	})
	if err != nil {
		t.Fatalf("NextAction failed: %v", err)
	}
	if action.Kind != domain.ActionCheck {
		t.Fatalf("expected check action, got %q", action.Kind)
	}
	if !strings.Contains(out.String(), "invalid action") {
		t.Fatalf("expected invalid action hint in output, got %q", out.String())
	}
	if !strings.Contains(out.String(), "POKER TABLE") {
		t.Fatalf("expected ASCII table header in prompt, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Street: flop") {
		t.Fatalf("expected street in prompt, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Hole: -- --") {
		t.Fatalf("expected hidden hole cards placeholder in prompt, got %q", out.String())
	}
	if !strings.Contains(out.String(), "To Call: 0") {
		t.Fatalf("expected to-call info in prompt, got %q", out.String())
	}
	if !strings.Contains(out.String(), "bet(b)") {
		t.Fatalf("expected bet option in prompt, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Options: fold(f)/check(k)/bet(b) <amt>") {
		t.Fatalf("expected short aliases in prompt, got %q", out.String())
	}
	if strings.Contains(out.String(), "call(c)") {
		t.Fatalf("did not expect call option when to_call=0, got %q", out.String())
	}
	if !strings.Contains(out.String(), "checked on flop") {
		t.Fatalf("expected checked street confirmation, got %q", out.String())
	}
}

func TestHumanProviderRejectsIllegalCheckWhenFacingBet(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	seat := mustSeatNo(t, cfg, 1)

	state := domain.HandState{
		ActingSeat: seat,
		CurrentBet: 100,
		MinRaiseTo: 200,
		Seats:      []domain.SeatState{{SeatNo: seat, Stack: 1000, CommittedInRound: 50}},
	}

	in := strings.NewReader("check\ncall\n")
	out := &strings.Builder{}
	provider := newHumanProvider(in, out)

	action, err := provider.NextAction(context.Background(), state)
	if err != nil {
		t.Fatalf("NextAction failed: %v", err)
	}
	if action.Kind != domain.ActionCall {
		t.Fatalf("expected call action, got %q", action.Kind)
	}
	if !strings.Contains(out.String(), "invalid action") {
		t.Fatalf("expected invalid action hint in output, got %q", out.String())
	}
	if !strings.Contains(out.String(), "raise(r) <amt>") {
		t.Fatalf("expected raise option in prompt, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Options: fold(f)/call(c)/raise(r) <amt>") {
		t.Fatalf("expected check hidden when to_call>0, got %q", out.String())
	}
}

func TestHumanProviderRejectsRaiseBelowMinimum(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	seat := mustSeatNo(t, cfg, 1)

	state := domain.HandState{
		ActingSeat: seat,
		CurrentBet: 100,
		MinRaiseTo: 250,
		Seats:      []domain.SeatState{{SeatNo: seat, Stack: 1000, CommittedInRound: 50}},
	}

	in := strings.NewReader("raise 150\nraise 300\n")
	out := &strings.Builder{}
	provider := newHumanProvider(in, out)

	action, err := provider.NextAction(context.Background(), state)
	if err != nil {
		t.Fatalf("NextAction failed: %v", err)
	}
	if action.Kind != domain.ActionRaise {
		t.Fatalf("expected raise action, got %q", action.Kind)
	}
	if action.Amount == nil || *action.Amount != 300 {
		t.Fatalf("expected raise amount 300, got %v", action.Amount)
	}
	if !strings.Contains(out.String(), "Min Raise To: 250") {
		t.Fatalf("expected min_raise_to hint in output, got %q", out.String())
	}
}

func TestHumanProviderPromptIncludesSeatAndBoardInfo(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	seat1 := mustSeatNo(t, cfg, 1)
	seat2 := mustSeatNo(t, cfg, 2)
	rankA, err := domain.NewRank(14)
	if err != nil {
		t.Fatalf("NewRank failed: %v", err)
	}
	rankK, err := domain.NewRank(13)
	if err != nil {
		t.Fatalf("NewRank failed: %v", err)
	}

	state := domain.HandState{
		HandNo:     42,
		TableID:    "local-table-1",
		ButtonSeat: seat1,
		ActingSeat: seat2,
		Street:     domain.StreetFlop,
		Pot:        120,
		CurrentBet: 100,
		MinRaiseTo: 160,
		Board: []domain.Card{
			domain.NewCard(rankA, domain.SuitSpades),
			domain.NewCard(rankK, domain.SuitHearts),
		},
		HoleCards: []domain.SeatCards{
			{
				SeatNo: seat2,
				Cards: []domain.Card{
					domain.NewCard(rankA, domain.SuitClubs),
					domain.NewCard(rankK, domain.SuitDiamonds),
				},
			},
		},
		Seats: []domain.SeatState{
			{SeatNo: seat1, Stack: 980, CommittedInRound: 20},
			{SeatNo: seat2, Stack: 900, CommittedInRound: 80},
		},
	}

	in := strings.NewReader("call\n")
	out := &strings.Builder{}
	provider := newHumanProvider(in, out)

	_, err = provider.NextAction(context.Background(), state)
	if err != nil {
		t.Fatalf("NextAction failed: %v", err)
	}

	rendered := out.String()
	if !strings.Contains(rendered, "Hand #42") {
		t.Fatalf("expected hand number in output, got %q", rendered)
	}
	if !strings.Contains(rendered, "Table: local-table-1") {
		t.Fatalf("expected table id in output, got %q", rendered)
	}
	if !strings.Contains(rendered, "BOARD: [A♠] [K♥] [--] [--] [--]") {
		t.Fatalf("expected board cards in table layout, got %q", rendered)
	}
	if !strings.Contains(rendered, "+------------------------------------------------------+") {
		t.Fatalf("expected table outline in output, got %q", rendered)
	}
	if !strings.Contains(rendered, "Hole: A♣ K♦") {
		t.Fatalf("expected acting seat hole cards in output, got %q", rendered)
	}
	if !strings.Contains(rendered, "D Seat 1") {
		t.Fatalf("expected dealer marker for seat 1, got %q", rendered)
	}
	if !strings.Contains(rendered, "-> A Seat 2") {
		t.Fatalf("expected acting marker for seat 2, got %q", rendered)
	}
	if !strings.Contains(rendered, "(BTN/SB)") {
		t.Fatalf("expected heads-up BTN/SB position label, got %q", rendered)
	}
	if !strings.Contains(rendered, "(BB)") {
		t.Fatalf("expected heads-up BB position label, got %q", rendered)
	}
}

func TestRenderMiniPokerTable_SixMaxShowsNamedPositions(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	state := domain.HandState{
		HandNo:     7,
		TableID:    "local-table-1",
		ButtonSeat: mustSeatNo(t, cfg, 1),
		ActingSeat: mustSeatNo(t, cfg, 4),
		Street:     domain.StreetPreflop,
		Pot:        150,
		CurrentBet: 100,
		MinRaiseTo: 200,
		Seats: []domain.SeatState{
			{SeatNo: mustSeatNo(t, cfg, 1), Stack: 10000},
			{SeatNo: mustSeatNo(t, cfg, 2), Stack: 10000},
			{SeatNo: mustSeatNo(t, cfg, 3), Stack: 10000},
			{SeatNo: mustSeatNo(t, cfg, 4), Stack: 10000},
			{SeatNo: mustSeatNo(t, cfg, 5), Stack: 10000},
			{SeatNo: mustSeatNo(t, cfg, 6), Stack: 10000},
		},
	}

	rendered := renderMiniPokerTable(state, 100, "fold(f)/call(c)/raise(r) <amt>")

	for _, position := range []string{"(BTN)", "(SB)", "(BB)", "(UTG)", "(HJ)", "(CO)"} {
		if !strings.Contains(rendered, position) {
			t.Fatalf("expected position %s in output, got %q", position, rendered)
		}
	}
	for _, layoutPosition := range []string{"S1(BTN)", "S2(SB)", "S3(BB)", "S4(UTG)", "S5(HJ)", "S6(CO)"} {
		if !strings.Contains(rendered, layoutPosition) {
			t.Fatalf("expected layout position %s in output, got %q", layoutPosition, rendered)
		}
	}
}

func TestHumanProvider_BareBetIsInvalidWhenFacingOpenBet(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	seat := mustSeatNo(t, cfg, 1)

	state := domain.HandState{
		ActingSeat: seat,
		CurrentBet: 100,
		MinRaiseTo: 250,
		Seats:      []domain.SeatState{{SeatNo: seat, Stack: 1000, CommittedInRound: 50}},
	}

	in := strings.NewReader("bet\nraise 250\n")
	out := &strings.Builder{}
	provider := newHumanProvider(in, out)

	action, err := provider.NextAction(context.Background(), state)
	if err != nil {
		t.Fatalf("NextAction failed: %v", err)
	}
	if action.Kind != domain.ActionRaise {
		t.Fatalf("expected raise action, got %q", action.Kind)
	}
	if action.Amount == nil || *action.Amount != 250 {
		t.Fatalf("expected raise amount 250, got %v", action.Amount)
	}
	if !strings.Contains(out.String(), "invalid action") {
		t.Fatalf("expected invalid action hint for bare bet, got %q", out.String())
	}
}

func TestHumanProvider_BareRBecomesMinimumRaiseWhenFacingOpenBet(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	seat := mustSeatNo(t, cfg, 1)

	state := domain.HandState{
		ActingSeat: seat,
		CurrentBet: 100,
		MinRaiseTo: 250,
		Seats:      []domain.SeatState{{SeatNo: seat, Stack: 1000, CommittedInRound: 50}},
	}

	in := strings.NewReader("r\n")
	out := &strings.Builder{}
	provider := newHumanProvider(in, out)

	action, err := provider.NextAction(context.Background(), state)
	if err != nil {
		t.Fatalf("NextAction failed: %v", err)
	}
	if action.Kind != domain.ActionRaise {
		t.Fatalf("expected raise action, got %q", action.Kind)
	}
	if action.Amount == nil || *action.Amount != 250 {
		t.Fatalf("expected raise amount 250, got %v", action.Amount)
	}
	if !strings.Contains(out.String(), "minimum raise to 250") {
		t.Fatalf("expected min-raise hint in output, got %q", out.String())
	}
}

func TestHumanProvider_BareBetBecomesMinimumBetWhenNoBetToCall(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	seat := mustSeatNo(t, cfg, 1)

	state := domain.HandState{
		ActingSeat: seat,
		CurrentBet: 0,
		MinRaiseTo: 200,
		Seats:      []domain.SeatState{{SeatNo: seat, Stack: 1000, CommittedInRound: 0}},
	}

	in := strings.NewReader("bet\n")
	out := &strings.Builder{}
	provider := newHumanProvider(in, out)

	action, err := provider.NextAction(context.Background(), state)
	if err != nil {
		t.Fatalf("NextAction failed: %v", err)
	}
	if action.Kind != domain.ActionBet {
		t.Fatalf("expected bet action, got %q", action.Kind)
	}
	if action.Amount == nil || *action.Amount != 200 {
		t.Fatalf("expected bet amount 200, got %v", action.Amount)
	}
	if !strings.Contains(out.String(), "minimum bet to 200") {
		t.Fatalf("expected min-bet hint in output, got %q", out.String())
	}
}

func TestSeatProviderPrintsHumanAndBotActions(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	humanSeat := mustSeatNo(t, cfg, 1)
	botSeat := mustSeatNo(t, cfg, 2)
	output := &strings.Builder{}

	provider := seatProvider{
		humanSeat: humanSeat,
		human:     newScriptActionProvider(t, domain.ActionCheck, nil),
		bot:       newScriptActionProvider(t, domain.ActionCall, nil),
		out:       output,
	}

	_, err := provider.NextAction(context.Background(), domain.HandState{ActingSeat: humanSeat})
	if err != nil {
		t.Fatalf("human NextAction failed: %v", err)
	}
	_, err = provider.NextAction(context.Background(), domain.HandState{ActingSeat: botSeat})
	if err != nil {
		t.Fatalf("bot NextAction failed: %v", err)
	}

	if !strings.Contains(output.String(), "you (seat 1) -> check") {
		t.Fatalf("expected human action output, got %q", output.String())
	}
	if !strings.Contains(output.String(), "bot (seat 2) -> call") {
		t.Fatalf("expected bot action output, got %q", output.String())
	}
}

func TestDeterministicProviderCallsWhenFacingBet(t *testing.T) {
	t.Parallel()

	cfg := domain.DefaultV0TableConfig()
	seat := mustSeatNo(t, cfg, 1)
	state := domain.HandState{
		ActingSeat: seat,
		CurrentBet: 100,
		Seats:      []domain.SeatState{{SeatNo: seat, CommittedInRound: 50}},
	}

	action, err := (deterministicProvider{}).NextAction(context.Background(), state)
	if err != nil {
		t.Fatalf("NextAction failed: %v", err)
	}
	if action.Kind != domain.ActionCall {
		t.Fatalf("expected call, got %q", action.Kind)
	}
}

func mustSeatNo(t *testing.T, cfg domain.TableConfig, value uint8) domain.SeatNo {
	t.Helper()
	seatNo, err := domain.NewSeatNo(value, cfg.MaxSeats)
	if err != nil {
		t.Fatalf("NewSeatNo failed: %v", err)
	}
	return seatNo
}

type scriptActionProvider struct {
	action domain.Action
}

func newScriptActionProvider(t *testing.T, kind domain.ActionKind, amount *uint32) scriptActionProvider {
	t.Helper()
	action, err := domain.NewAction(kind, amount)
	if err != nil {
		t.Fatalf("NewAction failed: %v", err)
	}
	return scriptActionProvider{action: action}
}

func (p scriptActionProvider) NextAction(context.Context, domain.HandState) (domain.Action, error) {
	return p.action, nil
}
