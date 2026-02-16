package agentclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

func TestBuildProtocolRequestMapsFields(t *testing.T) {
	t.Parallel()

	state := baseState(t)
	state.CurrentBet = 100
	state.MinRaiseTo = 250
	state.Seats[0].CommittedInRound = 50

	payload, legal, err := buildProtocolRequest(state, mustSeatNo(t, 1), 2000)
	if err != nil {
		t.Fatalf("buildProtocolRequest failed: %v", err)
	}

	if payload.ProtocolVersion != 1 {
		t.Fatalf("expected protocol 1, got %d", payload.ProtocolVersion)
	}
	if payload.Seat != 1 {
		t.Fatalf("expected seat 1, got %d", payload.Seat)
	}
	if len(payload.HoleCards) != 2 || payload.HoleCards[0] != "As" || payload.HoleCards[1] != "Td" {
		t.Fatalf("unexpected hole cards: %+v", payload.HoleCards)
	}
	if len(payload.Board) != 3 || payload.Board[0] != "2c" || payload.Board[2] != "7h" {
		t.Fatalf("unexpected board mapping: %+v", payload.Board)
	}
	if payload.ToCall != 50 {
		t.Fatalf("expected to_call 50, got %d", payload.ToCall)
	}
	if payload.MinRaiseTo == nil || *payload.MinRaiseTo != 250 {
		t.Fatalf("expected min_raise_to 250, got %v", payload.MinRaiseTo)
	}
	if payload.Stacks["1"] != state.Seats[0].Stack {
		t.Fatalf("expected stack map seat1=%d, got %d", state.Seats[0].Stack, payload.Stacks["1"])
	}
	if payload.Bets["1"] != 50 {
		t.Fatalf("expected bets seat1=50, got %d", payload.Bets["1"])
	}
	if _, ok := legal[domain.ActionRaise]; !ok {
		t.Fatalf("expected raise legal actions, got %+v", payload.LegalActions)
	}
}

func TestBuildProtocolRequestNoBetAddsCheckAndBet(t *testing.T) {
	t.Parallel()

	state := baseState(t)
	state.CurrentBet = 0
	state.Seats[0].CommittedInRound = 0

	payload, legal, err := buildProtocolRequest(state, mustSeatNo(t, 1), 2000)
	if err != nil {
		t.Fatalf("buildProtocolRequest failed: %v", err)
	}

	if _, ok := legal[domain.ActionCheck]; !ok {
		t.Fatal("expected check legal")
	}
	if _, ok := legal[domain.ActionBet]; !ok {
		t.Fatal("expected bet legal")
	}
	if _, ok := legal[domain.ActionCall]; ok {
		t.Fatal("did not expect call legal")
	}
	if payload.MinRaiseTo != nil {
		t.Fatalf("expected nil min_raise_to when raise not legal, got %v", *payload.MinRaiseTo)
	}
}

func TestBuildProtocolRequestAllInFacingBetNoRaise(t *testing.T) {
	t.Parallel()

	state := baseState(t)
	state.CurrentBet = 100
	state.Seats[0].CommittedInRound = 50
	state.Seats[0].Stack = 50

	_, legal, err := buildProtocolRequest(state, mustSeatNo(t, 1), 2000)
	if err != nil {
		t.Fatalf("buildProtocolRequest failed: %v", err)
	}

	if _, ok := legal[domain.ActionRaise]; ok {
		t.Fatal("did not expect raise legal when stack equals to_call")
	}
	if _, ok := legal[domain.ActionCall]; !ok {
		t.Fatal("expected call legal")
	}
}

func TestBuildProtocolRequestMissingHoleCardsFails(t *testing.T) {
	t.Parallel()

	state := baseState(t)
	state.HoleCards = nil

	_, _, err := buildProtocolRequest(state, mustSeatNo(t, 1), 2000)
	if !errors.Is(err, ErrMissingHoleCards) {
		t.Fatalf("expected ErrMissingHoleCards, got %v", err)
	}
}

func TestParseAndValidateProtocolResponse(t *testing.T) {
	t.Parallel()

	legal := map[domain.ActionKind]struct{}{
		domain.ActionFold:  {},
		domain.ActionCall:  {},
		domain.ActionRaise: {},
	}

	good, err := parseAndValidateProtocolResponse(protocolResponse{Action: "call"}, legal)
	if err != nil {
		t.Fatalf("expected valid response, got %v", err)
	}
	if good.Kind != domain.ActionCall {
		t.Fatalf("expected call kind, got %q", good.Kind)
	}

	amount := uint32(200)
	good, err = parseAndValidateProtocolResponse(protocolResponse{Action: "raise", Amount: &amount}, legal)
	if err != nil {
		t.Fatalf("expected valid raise response, got %v", err)
	}
	if good.Amount == nil || *good.Amount != 200 {
		t.Fatalf("expected raise amount 200, got %v", good.Amount)
	}

	_, err = parseAndValidateProtocolResponse(protocolResponse{Action: "dance"}, legal)
	if !errors.Is(err, ErrIllegalAgentAction) {
		t.Fatalf("expected ErrIllegalAgentAction, got %v", err)
	}

	_, err = parseAndValidateProtocolResponse(protocolResponse{Action: "raise"}, legal)
	if !errors.Is(err, ErrIllegalAgentAction) {
		t.Fatalf("expected ErrIllegalAgentAction for missing raise amount, got %v", err)
	}
}

func TestClientNextActionHappyPath(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		defer r.Body.Close()

		var payload protocolRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request payload: %v", err)
		}
		if payload.ProtocolVersion != 1 {
			t.Fatalf("expected protocol version 1, got %d", payload.ProtocolVersion)
		}
		if payload.HoleCards[0] != "As" {
			t.Fatalf("expected As hole card, got %+v", payload.HoleCards)
		}
		_ = json.NewEncoder(w).Encode(protocolResponse{Action: "check"})
	}))
	defer server.Close()

	state := baseState(t)
	client := New(2 * time.Second)
	action, err := client.NextAction(context.Background(), Request{
		EndpointURL:     server.URL,
		State:           state,
		ActingSeat:      mustSeatNo(t, 1),
		ActionTimeoutMS: 2000,
	})
	if err != nil {
		t.Fatalf("NextAction failed: %v", err)
	}
	if action.Kind != domain.ActionCheck {
		t.Fatalf("expected check action, got %q", action.Kind)
	}
}

func TestClientNextActionTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(protocolResponse{Action: "check"})
	}))
	defer server.Close()

	state := baseState(t)
	client := New(5 * time.Millisecond)
	_, err := client.NextAction(context.Background(), Request{
		EndpointURL:     server.URL,
		State:           state,
		ActingSeat:      mustSeatNo(t, 1),
		ActionTimeoutMS: 2000,
	})
	if !errors.Is(err, ErrRequestTimeout) {
		t.Fatalf("expected ErrRequestTimeout, got %v", err)
	}
}

func TestClientNextActionMalformedResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{not-json"))
	}))
	defer server.Close()

	state := baseState(t)
	client := New(2 * time.Second)
	_, err := client.NextAction(context.Background(), Request{
		EndpointURL:     server.URL,
		State:           state,
		ActingSeat:      mustSeatNo(t, 1),
		ActionTimeoutMS: 2000,
	})
	if !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse, got %v", err)
	}
}

func TestClientNextActionRejectsTrailingResponseData(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"action":"check"} {"extra":true}`))
	}))
	defer server.Close()

	state := baseState(t)
	client := New(2 * time.Second)
	_, err := client.NextAction(context.Background(), Request{
		EndpointURL:     server.URL,
		State:           state,
		ActingSeat:      mustSeatNo(t, 1),
		ActionTimeoutMS: 2000,
	})
	if !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse, got %v", err)
	}
}

func TestClientNextActionNon200Status(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()

	state := baseState(t)
	client := New(2 * time.Second)
	_, err := client.NextAction(context.Background(), Request{
		EndpointURL:     server.URL,
		State:           state,
		ActingSeat:      mustSeatNo(t, 1),
		ActionTimeoutMS: 2000,
	})
	if !errors.Is(err, ErrNetwork) {
		t.Fatalf("expected ErrNetwork, got %v", err)
	}
}

func TestClientNextActionIllegalAgentAction(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(protocolResponse{Action: "bet", Amount: u32ptr(100)})
	}))
	defer server.Close()

	state := baseState(t)
	state.CurrentBet = 100
	state.Seats[0].CommittedInRound = 50

	client := New(2 * time.Second)
	_, err := client.NextAction(context.Background(), Request{
		EndpointURL:     server.URL,
		State:           state,
		ActingSeat:      mustSeatNo(t, 1),
		ActionTimeoutMS: 2000,
	})
	if !errors.Is(err, ErrIllegalAgentAction) {
		t.Fatalf("expected ErrIllegalAgentAction, got %v", err)
	}
}

func baseState(t *testing.T) domain.HandState {
	t.Helper()
	seat1 := mustSeatNo(t, 1)
	seat2 := mustSeatNo(t, 2)
	return domain.HandState{
		HandID:     "hand-1",
		TableID:    "table-1",
		ActingSeat: seat1,
		Pot:        150,
		CurrentBet: 0,
		MinRaiseTo: 200,
		Board: []domain.Card{
			mustCard(t, 2, domain.SuitClubs),
			mustCard(t, 3, domain.SuitDiamonds),
			mustCard(t, 7, domain.SuitHearts),
		},
		HoleCards: []domain.SeatCards{
			{SeatNo: seat1, Cards: []domain.Card{mustCard(t, 14, domain.SuitSpades), mustCard(t, 10, domain.SuitDiamonds)}},
			{SeatNo: seat2, Cards: []domain.Card{mustCard(t, 13, domain.SuitClubs), mustCard(t, 12, domain.SuitHearts)}},
		},
		Seats: []domain.SeatState{
			{SeatNo: seat1, Stack: 9950, CommittedInRound: 0, Status: domain.SeatStatusActive},
			{SeatNo: seat2, Stack: 9900, CommittedInRound: 100, Status: domain.SeatStatusActive},
		},
	}
}

func mustSeatNo(t *testing.T, n uint8) domain.SeatNo {
	t.Helper()
	seat, err := domain.NewSeatNo(n, domain.DefaultMaxSeats)
	if err != nil {
		t.Fatalf("NewSeatNo failed: %v", err)
	}
	return seat
}

func mustCard(t *testing.T, rank uint8, suit domain.Suit) domain.Card {
	t.Helper()
	r, err := domain.NewRank(rank)
	if err != nil {
		t.Fatalf("NewRank failed: %v", err)
	}
	return domain.NewCard(r, suit)
}

func u32ptr(v uint32) *uint32 {
	return &v
}
