package persistence

import (
	"errors"
	"testing"
	"time"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

func runRepositoryResourcesContractTests(t *testing.T, mkRepo func(t *testing.T) Repository) {
	t.Helper()

	t.Run("Contract_CreateAndGetUser", func(t *testing.T) {
		repo := mkRepo(t)
		rec := UserRecord{ID: "u1", Name: "user", Token: "tok", CreatedAt: time.Now().UTC()}
		if err := repo.CreateUser(rec); err != nil {
			t.Fatalf("CreateUser failed: %v", err)
		}
	})

	t.Run("Contract_CreateAgentRequiresUser", func(t *testing.T) {
		repo := mkRepo(t)
		if err := repo.CreateAgent(AgentRecord{
			ID:        "a1",
			UserID:    "missing",
			Name:      "agent",
			CreatedAt: time.Now().UTC(),
		}); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("expected ErrUserNotFound, got %v", err)
		}
		if err := repo.CreateUser(UserRecord{ID: "u1", Name: "user", Token: "tok", CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatalf("CreateUser failed: %v", err)
		}
		if err := repo.CreateAgent(AgentRecord{
			ID:        "a1",
			UserID:    "u1",
			Name:      "agent",
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("CreateAgent failed: %v", err)
		}
	})

	t.Run("Contract_CreateVersionRequiresAgent", func(t *testing.T) {
		repo := mkRepo(t)
		if err := repo.CreateAgentVersion(AgentVersionRecord{
			ID:          "v1",
			AgentID:     "missing",
			Version:     1,
			EndpointURL: "http://agent",
			CreatedAt:   time.Now().UTC(),
		}); !errors.Is(err, ErrAgentNotFound) {
			t.Fatalf("expected ErrAgentNotFound, got %v", err)
		}
		if err := repo.CreateUser(UserRecord{ID: "u1", Name: "user", Token: "tok", CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatalf("CreateUser failed: %v", err)
		}
		if err := repo.CreateAgent(AgentRecord{ID: "a1", UserID: "u1", Name: "agent", CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatalf("CreateAgent failed: %v", err)
		}
		if err := repo.CreateAgentVersion(AgentVersionRecord{
			ID:          "v1",
			AgentID:     "a1",
			Version:     1,
			EndpointURL: "http://agent",
			CreatedAt:   time.Now().UTC(),
		}); err != nil {
			t.Fatalf("CreateAgentVersion failed: %v", err)
		}
	})

	t.Run("Contract_CreateTableAndUpsertListSeatsOrdered", func(t *testing.T) {
		repo := mkRepo(t)
		if err := repo.CreateUser(UserRecord{ID: "u1", Name: "user", Token: "tok", CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatalf("CreateUser failed: %v", err)
		}
		if err := repo.CreateAgent(AgentRecord{ID: "a1", UserID: "u1", Name: "agent1", CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatalf("CreateAgent failed: %v", err)
		}
		if err := repo.CreateAgent(AgentRecord{ID: "a2", UserID: "u1", Name: "agent2", CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatalf("CreateAgent failed: %v", err)
		}
		if err := repo.CreateAgentVersion(AgentVersionRecord{ID: "v1", AgentID: "a1", Version: 1, EndpointURL: "http://a1", CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatalf("CreateAgentVersion v1 failed: %v", err)
		}
		if err := repo.CreateAgentVersion(AgentVersionRecord{ID: "v2", AgentID: "a2", Version: 1, EndpointURL: "http://a2", CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatalf("CreateAgentVersion v2 failed: %v", err)
		}
		if err := repo.CreateTable(TableRecord{
			ID:         "t1",
			Name:       "table",
			MaxSeats:   6,
			SmallBlind: 50,
			BigBlind:   100,
			Status:     "idle",
			CreatedAt:  time.Now().UTC(),
		}); err != nil {
			t.Fatalf("CreateTable failed: %v", err)
		}
		if err := repo.UpsertSeat(SeatRecord{
			ID:             "s2",
			TableID:        "t1",
			SeatNo:         2,
			AgentID:        "a2",
			AgentVersionID: "v2",
			Stack:          9000,
			Status:         domain.SeatStatusActive,
			CreatedAt:      time.Now().UTC(),
		}); err != nil {
			t.Fatalf("UpsertSeat s2 failed: %v", err)
		}
		if err := repo.UpsertSeat(SeatRecord{
			ID:             "s1",
			TableID:        "t1",
			SeatNo:         1,
			AgentID:        "a1",
			AgentVersionID: "v1",
			Stack:          10000,
			Status:         domain.SeatStatusActive,
			CreatedAt:      time.Now().UTC(),
		}); err != nil {
			t.Fatalf("UpsertSeat s1 failed: %v", err)
		}
		seats, err := repo.ListSeats("t1")
		if err != nil {
			t.Fatalf("ListSeats failed: %v", err)
		}
		if len(seats) != 2 || seats[0].SeatNo != 1 || seats[1].SeatNo != 2 {
			t.Fatalf("expected ordered seats [1,2], got %+v", seats)
		}
	})

	t.Run("Contract_SeatRequiresExistingForeignKeys", func(t *testing.T) {
		repo := mkRepo(t)
		if err := repo.UpsertSeat(SeatRecord{
			ID:             "s1",
			TableID:        "missing",
			SeatNo:         1,
			AgentID:        "a1",
			AgentVersionID: "v1",
			Stack:          10000,
			Status:         domain.SeatStatusActive,
			CreatedAt:      time.Now().UTC(),
		}); !errors.Is(err, ErrTableNotFound) {
			t.Fatalf("expected ErrTableNotFound, got %v", err)
		}
	})
}
