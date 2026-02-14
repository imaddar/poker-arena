package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	"github.com/imaddar/poker-arena/services/engine/internal/persistence"
	"github.com/imaddar/poker-arena/services/engine/internal/tablerunner"
)

func TestStartRunPersistsHandStartedAtIndependentlyFromRunStartedAt(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(
		repo,
		func(_ tablerunner.ActionProvider, cfg tablerunner.RunnerConfig) runnerLike {
			return fakeRunner{cfg: cfg}
		},
		func(tableID string) (tablerunner.ActionProvider, error) {
			return fakeProvider{}, nil
		},
	)

	req := httptest.NewRequest(http.MethodPost, "/tables/table-1/start", strings.NewReader(`{
		"hands_to_run": 1,
		"seats": [
			{"seat_no": 1, "stack": 10000, "status": "active"},
			{"seat_no": 2, "stack": 10000, "status": "active"}
		]
	}`))
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	waitForTableRunStatus(t, repo, "table-1", persistence.TableRunStatusCompleted)

	runRecord, ok, err := repo.GetTableRun("table-1")
	if err != nil {
		t.Fatalf("GetTableRun failed: %v", err)
	}
	if !ok {
		t.Fatal("expected run record")
	}

	hands, err := repo.ListHands("table-1")
	if err != nil {
		t.Fatalf("ListHands failed: %v", err)
	}
	if len(hands) != 1 {
		t.Fatalf("expected one hand, got %d", len(hands))
	}
	if hands[0].StartedAt.Equal(runRecord.StartedAt) {
		t.Fatalf("expected hand start time (%s) to differ from run start time (%s)", hands[0].StartedAt, runRecord.StartedAt)
	}
	if hands[0].StartedAt.Before(runRecord.StartedAt) {
		t.Fatalf("expected hand start time (%s) not before run start (%s)", hands[0].StartedAt, runRecord.StartedAt)
	}
}

type fakeRunner struct {
	cfg tablerunner.RunnerConfig
}

func (r fakeRunner) RunTable(_ context.Context, input tablerunner.RunTableInput) (tablerunner.RunTableResult, error) {
	seat1, err := domain.NewSeatNo(1, input.Config.MaxSeats)
	if err != nil {
		return tablerunner.RunTableResult{}, err
	}
	seat2, err := domain.NewSeatNo(2, input.Config.MaxSeats)
	if err != nil {
		return tablerunner.RunTableResult{}, err
	}

	initial := domain.HandState{
		HandID: "hand-1",
		TableID: input.TableID,
		HandNo: input.StartingHand,
		Phase: domain.HandPhaseBetting,
		Street: domain.StreetPreflop,
		ActingSeat: seat1,
		Seats: []domain.SeatState{
			domain.NewSeatState(seat1, input.Config.StartingStack),
			domain.NewSeatState(seat2, input.Config.StartingStack),
		},
	}
	if r.cfg.OnHandStart != nil {
		r.cfg.OnHandStart(tablerunner.RunHandInput{
			TableID: input.TableID,
			HandNo: input.StartingHand,
			ButtonSeat: input.ButtonSeat,
			Seats: append([]domain.SeatState(nil), input.Seats...),
			Config: input.Config,
		}, initial)
	}

	time.Sleep(20 * time.Millisecond)
	final := initial
	final.Phase = domain.HandPhaseComplete
	final.ShowdownAwards = []domain.PotAward{{
		Amount: input.Config.SmallBlind + input.Config.BigBlind,
		Seats: []domain.SeatNo{seat1},
		Reason: "uncontested",
	}}
	if r.cfg.OnHandComplete != nil {
		r.cfg.OnHandComplete(tablerunner.HandSummary{
			HandNo: input.StartingHand,
			FinalPhase: domain.HandPhaseComplete,
			ActionCount: 0,
			FallbackCount: 0,
			FinalState: final,
		})
	}

	return tablerunner.RunTableResult{
		HandsCompleted: 1,
		FinalButton: input.ButtonSeat,
		FinalSeats: append([]domain.SeatState(nil), input.Seats...),
		TotalActions: 0,
		TotalFallbacks: 0,
		HandSummaries: []tablerunner.HandSummary{{
			HandNo: input.StartingHand,
			FinalPhase: domain.HandPhaseComplete,
			ActionCount: 0,
			FallbackCount: 0,
			FinalState: final,
		}},
	}, nil
}

type fakeProvider struct{}

func (fakeProvider) NextAction(_ context.Context, _ domain.HandState) (domain.Action, error) {
	return domain.Action{}, fmt.Errorf("unexpected provider call")
}

func waitForTableRunStatus(t *testing.T, repo persistence.Repository, tableID string, want persistence.TableRunStatus) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		record, ok, err := repo.GetTableRun(tableID)
		if err != nil {
			t.Fatalf("GetTableRun failed: %v", err)
		}
		if ok && record.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	record, ok, err := repo.GetTableRun(tableID)
	if err != nil {
		t.Fatalf("GetTableRun failed: %v", err)
	}
	if !ok {
		t.Fatalf("table run %s not found while waiting for status %q", tableID, want)
	}
	body, _ := json.Marshal(record)
	t.Fatalf("timed out waiting for table %s to reach status %q; latest=%s", tableID, want, string(body))
}
