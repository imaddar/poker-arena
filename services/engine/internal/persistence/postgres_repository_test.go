package persistence

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	_ "github.com/lib/pq"
)

func TestPostgresRepository_Contract(t *testing.T) {
	runRepositoryContractTests(t, func(t *testing.T) Repository {
		t.Helper()
		db := openTestPostgresDB(t)
		repo := NewPostgresRepository(db)
		resetPostgresTables(t, db)
		return repo
	})
}

func TestPostgresRepository_ResourcesContract(t *testing.T) {
	runRepositoryResourcesContractTests(t, func(t *testing.T) Repository {
		t.Helper()
		db := openTestPostgresDB(t)
		repo := NewPostgresRepository(db)
		resetPostgresTables(t, db)
		return repo
	})
}

func TestPostgresRepository_CreateHandDuplicateReturnsErrHandAlreadyExists(t *testing.T) {
	db := openTestPostgresDB(t)
	repo := NewPostgresRepository(db)

	record := HandRecord{HandID: "dup-hand", TableID: "table-1", HandNo: 1, StartedAt: time.Now().UTC()}
	if err := repo.UpsertTableRun(TableRunRecord{
		TableID:        "table-1",
		Status:         TableRunStatusRunning,
		StartedAt:      time.Now().UTC(),
		HandsRequested: 1,
		CurrentHandNo:  1,
	}); err != nil {
		t.Fatalf("UpsertTableRun failed: %v", err)
	}
	if err := repo.CreateHand(record); err != nil {
		t.Fatalf("CreateHand first insert failed: %v", err)
	}
	err := repo.CreateHand(record)
	if !errors.Is(err, ErrHandAlreadyExists) {
		t.Fatalf("expected ErrHandAlreadyExists, got %v", err)
	}
}

func TestPostgresRepository_CompleteMissingHandReturnsErrHandNotFound(t *testing.T) {
	db := openTestPostgresDB(t)
	repo := NewPostgresRepository(db)

	err := repo.CompleteHand("missing", HandRecord{
		HandID:     "missing",
		TableID:    "table-1",
		HandNo:     1,
		StartedAt:  time.Now().UTC(),
		FinalState: domain.HandState{HandID: "missing", TableID: "table-1", HandNo: 1},
	})
	if !errors.Is(err, ErrHandNotFound) {
		t.Fatalf("expected ErrHandNotFound, got %v", err)
	}
}

func TestPostgresRepository_JSONBRoundTripFinalStateAndWinnerSummary(t *testing.T) {
	db := openTestPostgresDB(t)
	repo := NewPostgresRepository(db)
	now := time.Now().UTC()
	if err := repo.UpsertTableRun(TableRunRecord{
		TableID:        "table-1",
		Status:         TableRunStatusRunning,
		StartedAt:      now,
		HandsRequested: 1,
		CurrentHandNo:  1,
	}); err != nil {
		t.Fatalf("UpsertTableRun failed: %v", err)
	}

	if err := repo.CreateHand(HandRecord{
		HandID:    "h1",
		TableID:   "table-1",
		HandNo:    1,
		StartedAt: now,
	}); err != nil {
		t.Fatalf("CreateHand failed: %v", err)
	}

	ended := now.Add(time.Minute)
	final := HandRecord{
		HandID:     "h1",
		TableID:    "table-1",
		HandNo:     1,
		StartedAt:  now,
		EndedAt:    &ended,
		FinalPhase: domain.HandPhaseComplete,
		FinalState: domain.HandState{
			HandID: "h1", TableID: "table-1", HandNo: 1, Phase: domain.HandPhaseComplete,
			ShowdownAwards: []domain.PotAward{{Amount: 200, Seats: []domain.SeatNo{1}, Reason: "showdown"}},
		},
		WinnerSummary: []domain.PotAward{{Amount: 200, Seats: []domain.SeatNo{1}, Reason: "showdown"}},
	}
	if err := repo.CompleteHand("h1", final); err != nil {
		t.Fatalf("CompleteHand failed: %v", err)
	}

	hands, err := repo.ListHands("table-1")
	if err != nil {
		t.Fatalf("ListHands failed: %v", err)
	}
	if len(hands) != 1 {
		t.Fatalf("expected one hand, got %d", len(hands))
	}
	if hands[0].FinalState.HandID != "h1" {
		t.Fatalf("expected final state hand id h1, got %q", hands[0].FinalState.HandID)
	}
	if len(hands[0].WinnerSummary) != 1 || hands[0].WinnerSummary[0].Amount != 200 {
		t.Fatalf("unexpected winner summary: %+v", hands[0].WinnerSummary)
	}
}

func TestPostgresRepository_AppendActionMissingHandReturnsErrHandNotFound(t *testing.T) {
	db := openTestPostgresDB(t)
	repo := NewPostgresRepository(db)

	err := repo.AppendAction(ActionRecord{
		HandID:     "missing",
		Street:     domain.StreetPreflop,
		ActingSeat: 1,
		Action:     domain.ActionCheck,
		At:         time.Now().UTC(),
	})
	if !errors.Is(err, ErrHandNotFound) {
		t.Fatalf("expected ErrHandNotFound, got %v", err)
	}
}

func openTestPostgresDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("PingContext failed: %v", err)
	}
	if err := MigratePostgres(ctx, db); err != nil {
		t.Fatalf("MigratePostgres failed: %v", err)
	}
	resetPostgresTables(t, db)

	return db
}

func resetPostgresTables(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `TRUNCATE TABLE actions, hands, table_runs, seats, tables, agent_versions, agents, users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate tables failed: %v", err)
	}
}
