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

type staticEndpoints struct {
	bySeat map[domain.SeatNo]string
	err    error
}

func (s staticEndpoints) EndpointForSeat(_ domain.HandState, seat domain.SeatNo) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.bySeat[seat], nil
}

func TestActionProviderNextActionHappyPath(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(protocolResponse{Action: "check"})
	}))
	defer server.Close()

	state := baseState(t)
	provider := ActionProvider{
		Client:    New(2 * time.Second),
		Endpoints: staticEndpoints{bySeat: map[domain.SeatNo]string{mustSeatNo(t, 1): server.URL}},
	}

	action, err := provider.NextAction(context.Background(), state)
	if err != nil {
		t.Fatalf("NextAction failed: %v", err)
	}
	if action.Kind != domain.ActionCheck {
		t.Fatalf("expected check action, got %q", action.Kind)
	}
}

func TestActionProviderNextActionMissingEndpoint(t *testing.T) {
	t.Parallel()

	state := baseState(t)
	provider := ActionProvider{
		Client:    New(2 * time.Second),
		Endpoints: staticEndpoints{bySeat: map[domain.SeatNo]string{}},
	}

	_, err := provider.NextAction(context.Background(), state)
	if !errors.Is(err, ErrEndpointNotConfigured) {
		t.Fatalf("expected ErrEndpointNotConfigured, got %v", err)
	}
}

func TestActionProviderNextActionEndpointProviderError(t *testing.T) {
	t.Parallel()

	state := baseState(t)
	provider := ActionProvider{
		Client:    New(2 * time.Second),
		Endpoints: staticEndpoints{err: errors.New("lookup failed")},
	}

	_, err := provider.NextAction(context.Background(), state)
	if !errors.Is(err, ErrEndpointNotConfigured) {
		t.Fatalf("expected ErrEndpointNotConfigured, got %v", err)
	}
}

func TestActionProviderPropagatesClientError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()

	state := baseState(t)
	provider := ActionProvider{
		Client:    New(2 * time.Second),
		Endpoints: staticEndpoints{bySeat: map[domain.SeatNo]string{mustSeatNo(t, 1): server.URL}},
	}

	_, err := provider.NextAction(context.Background(), state)
	if !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse, got %v", err)
	}
}
