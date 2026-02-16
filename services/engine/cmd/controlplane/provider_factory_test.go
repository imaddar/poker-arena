package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/imaddar/poker-arena/services/engine/internal/agentclient"
	"github.com/imaddar/poker-arena/services/engine/internal/api"
	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

func TestNewProviderFactory_ResolvesSeatEndpointAndReturnsAction(t *testing.T) {
	t.Parallel()

	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"action": "check"})
	}))
	defer agent.Close()

	factory := newProviderFactory(2 * time.Second)
	provider, err := factory("table-1", api.StartRequest{
		Seats: []api.StartSeat{
			{SeatNo: 1, Stack: 10_000, Status: domain.SeatStatusActive, AgentEndpoint: agent.URL},
			{SeatNo: 2, Stack: 10_000, Status: domain.SeatStatusActive, AgentEndpoint: agent.URL},
		},
	}, api.ServerConfig{DefaultAgentTimeoutMS: 2000})
	if err != nil {
		t.Fatalf("factory failed: %v", err)
	}

	action, err := provider.NextAction(context.Background(), buildProviderTestState(t, mustSeatNo(t, 1)))
	if err != nil {
		t.Fatalf("NextAction failed: %v", err)
	}
	if action.Kind != domain.ActionCheck {
		t.Fatalf("expected check action, got %s", action.Kind)
	}
}

func TestNewProviderFactory_MissingActingSeatEndpointReturnsTypedError(t *testing.T) {
	t.Parallel()

	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"action": "check"})
	}))
	defer agent.Close()

	factory := newProviderFactory(2 * time.Second)
	provider, err := factory("table-1", api.StartRequest{
		Seats: []api.StartSeat{
			{SeatNo: 1, Stack: 10_000, Status: domain.SeatStatusActive, AgentEndpoint: agent.URL},
		},
	}, api.ServerConfig{DefaultAgentTimeoutMS: 2000})
	if err != nil {
		t.Fatalf("factory failed: %v", err)
	}

	_, err = provider.NextAction(context.Background(), buildProviderTestState(t, mustSeatNo(t, 2)))
	if !errors.Is(err, agentclient.ErrEndpointNotConfigured) {
		t.Fatalf("expected ErrEndpointNotConfigured, got %v", err)
	}
}

func TestNewProviderFactory_UsesPerSeatTimeoutOverride(t *testing.T) {
	t.Parallel()

	var capturedDeadline uint64
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ActionDeadlineMS uint64 `json:"action_deadline_ms"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		capturedDeadline = body.ActionDeadlineMS
		_ = json.NewEncoder(w).Encode(map[string]any{"action": "check"})
	}))
	defer agent.Close()

	seatTimeout := uint64(7500)
	factory := newProviderFactory(2 * time.Second)
	provider, err := factory("table-1", api.StartRequest{
		Seats: []api.StartSeat{
			{SeatNo: 1, Stack: 10_000, Status: domain.SeatStatusActive, AgentEndpoint: agent.URL, AgentTimeoutMS: &seatTimeout},
			{SeatNo: 2, Stack: 10_000, Status: domain.SeatStatusActive, AgentEndpoint: agent.URL},
		},
	}, api.ServerConfig{DefaultAgentTimeoutMS: 2000})
	if err != nil {
		t.Fatalf("factory failed: %v", err)
	}

	_, err = provider.NextAction(context.Background(), buildProviderTestState(t, mustSeatNo(t, 1)))
	if err != nil {
		t.Fatalf("NextAction failed: %v", err)
	}
	if capturedDeadline != seatTimeout {
		t.Fatalf("expected action_deadline_ms %d, got %d", seatTimeout, capturedDeadline)
	}
}

func buildProviderTestState(t *testing.T, actingSeat domain.SeatNo) domain.HandState {
	t.Helper()
	return domain.HandState{
		HandID:     "hand-1",
		TableID:    "table-1",
		HandNo:     1,
		Street:     domain.StreetPreflop,
		ActingSeat: actingSeat,
		CurrentBet: 0,
		MinRaiseTo: 100,
		Pot:        150,
		Seats: []domain.SeatState{
			domain.NewSeatState(mustSeatNo(t, 1), 10_000),
			domain.NewSeatState(mustSeatNo(t, 2), 10_000),
		},
		HoleCards: []domain.SeatCards{
			{SeatNo: mustSeatNo(t, 1), Cards: []domain.Card{mkCard(14, domain.SuitSpades), mkCard(13, domain.SuitSpades)}},
			{SeatNo: mustSeatNo(t, 2), Cards: []domain.Card{mkCard(2, domain.SuitClubs), mkCard(7, domain.SuitDiamonds)}},
		},
	}
}

func mustSeatNo(t *testing.T, seat uint8) domain.SeatNo {
	t.Helper()
	seatNo, err := domain.NewSeatNo(seat, domain.DefaultMaxSeats)
	if err != nil {
		t.Fatalf("NewSeatNo failed: %v", err)
	}
	return seatNo
}

func mkCard(rank uint8, suit domain.Suit) domain.Card {
	return domain.NewCard(domain.Rank(rank), suit)
}
