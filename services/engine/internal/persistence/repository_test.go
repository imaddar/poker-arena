package persistence

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

func TestInMemoryRepository_Contract(t *testing.T) {
	t.Parallel()
	runRepositoryContractTests(t, func(t *testing.T) Repository {
		t.Helper()
		return NewInMemoryRepository()
	})
}

func TestInMemoryRepository_ResourcesContract(t *testing.T) {
	t.Parallel()
	runRepositoryResourcesContractTests(t, func(t *testing.T) Repository {
		t.Helper()
		return NewInMemoryRepository()
	})
}

func TestInMemoryRepository_CreateAndListHandsOrderedByHandNo(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryRepository()
	now := time.Now().UTC()

	if err := repo.CreateHand(HandRecord{
		HandID:    "h2",
		TableID:   "t1",
		HandNo:    2,
		StartedAt: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateHand h2 failed: %v", err)
	}
	if err := repo.CreateHand(HandRecord{
		HandID:    "h1",
		TableID:   "t1",
		HandNo:    1,
		StartedAt: now.Add(1 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateHand h1 failed: %v", err)
	}

	hands, err := repo.ListHands("t1")
	if err != nil {
		t.Fatalf("ListHands failed: %v", err)
	}
	if len(hands) != 2 {
		t.Fatalf("expected 2 hands, got %d", len(hands))
	}
	if hands[0].HandNo != 1 || hands[1].HandNo != 2 {
		t.Fatalf("expected sorted hand numbers [1,2], got [%d,%d]", hands[0].HandNo, hands[1].HandNo)
	}
}

func TestInMemoryRepository_AppendAndListActionsPreserveInsertionOrder(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryRepository()
	if err := repo.CreateHand(HandRecord{HandID: "h1", TableID: "t1", HandNo: 1, StartedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("CreateHand failed: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := repo.AppendAction(ActionRecord{
			HandID:     "h1",
			ActingSeat: 1,
			Action:     domain.ActionKind(fmt.Sprintf("a%d", i)),
			At:         time.Now().UTC().Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("AppendAction %d failed: %v", i, err)
		}
	}

	actions, err := repo.ListActions("h1")
	if err != nil {
		t.Fatalf("ListActions failed: %v", err)
	}
	if len(actions) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(actions))
	}
	for i := range actions {
		want := domain.ActionKind(fmt.Sprintf("a%d", i))
		if actions[i].Action != want {
			t.Fatalf("expected action %q at index %d, got %q", want, i, actions[i].Action)
		}
	}
}

func TestInMemoryRepository_CompleteHandUpdatesFinalState(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryRepository()
	started := time.Now().UTC()
	if err := repo.CreateHand(HandRecord{
		HandID:    "h1",
		TableID:   "t1",
		HandNo:    1,
		StartedAt: started,
	}); err != nil {
		t.Fatalf("CreateHand failed: %v", err)
	}

	ended := started.Add(time.Minute)
	final := HandRecord{
		HandID:     "h1",
		TableID:    "t1",
		HandNo:     1,
		StartedAt:  started,
		EndedAt:    &ended,
		FinalPhase: domain.HandPhaseComplete,
		FinalState: domain.HandState{
			HandID:         "h1",
			TableID:        "t1",
			HandNo:         1,
			Phase:          domain.HandPhaseComplete,
			ShowdownAwards: []domain.PotAward{{Amount: 100, Seats: []domain.SeatNo{1}}},
		},
		WinnerSummary: []domain.PotAward{{Amount: 100, Seats: []domain.SeatNo{1}, Reason: "showdown"}},
	}
	if err := repo.CompleteHand("h1", final); err != nil {
		t.Fatalf("CompleteHand failed: %v", err)
	}

	hands, err := repo.ListHands("t1")
	if err != nil {
		t.Fatalf("ListHands failed: %v", err)
	}
	if len(hands) != 1 {
		t.Fatalf("expected one hand, got %d", len(hands))
	}
	if hands[0].EndedAt == nil {
		t.Fatal("expected EndedAt to be set")
	}
	if hands[0].FinalPhase != domain.HandPhaseComplete {
		t.Fatalf("expected final phase complete, got %q", hands[0].FinalPhase)
	}
	if len(hands[0].WinnerSummary) != 1 {
		t.Fatalf("expected winner summary length 1, got %d", len(hands[0].WinnerSummary))
	}
}

func TestInMemoryRepository_UpsertAndGetTableRun(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryRepository()
	started := time.Now().UTC()
	record := TableRunRecord{
		TableID:        "table-1",
		Status:         TableRunStatusRunning,
		StartedAt:      started,
		HandsRequested: 10,
		CurrentHandNo:  2,
	}
	if err := repo.UpsertTableRun(record); err != nil {
		t.Fatalf("UpsertTableRun failed: %v", err)
	}

	got, ok, err := repo.GetTableRun("table-1")
	if err != nil {
		t.Fatalf("GetTableRun failed: %v", err)
	}
	if !ok {
		t.Fatal("expected table run to exist")
	}
	if got.Status != TableRunStatusRunning {
		t.Fatalf("expected running status, got %q", got.Status)
	}

	ended := started.Add(time.Minute)
	record.Status = TableRunStatusCompleted
	record.EndedAt = &ended
	if err := repo.UpsertTableRun(record); err != nil {
		t.Fatalf("UpsertTableRun update failed: %v", err)
	}
	got, ok, err = repo.GetTableRun("table-1")
	if err != nil {
		t.Fatalf("GetTableRun update failed: %v", err)
	}
	if !ok || got.EndedAt == nil || got.Status != TableRunStatusCompleted {
		t.Fatalf("expected completed table run with EndedAt, got status=%q endedAtNil=%t", got.Status, got.EndedAt == nil)
	}
}

func TestInMemoryRepository_AppendActionRequiresExistingHand(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryRepository()
	err := repo.AppendAction(ActionRecord{
		HandID:     "missing",
		ActingSeat: 1,
		Action:     domain.ActionCheck,
		At:         time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected error for missing hand")
	}
	if err != ErrHandNotFound {
		t.Fatalf("expected ErrHandNotFound, got %v", err)
	}
}
func TestInMemoryRepository_ConcurrentAppendAndReadIsSafe(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryRepository()
	if err := repo.CreateHand(HandRecord{HandID: "h1", TableID: "table-1", HandNo: 1, StartedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("CreateHand failed: %v", err)
	}
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = repo.AppendAction(ActionRecord{
				HandID:     "h1",
				ActingSeat: 1,
				Action:     domain.ActionCall,
				At:         time.Now().UTC().Add(time.Duration(i) * time.Millisecond),
			})
		}(i)
	}

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = repo.GetTableRun("table-1")
			_, _ = repo.ListActions("h1")
		}()
	}

	wg.Wait()

	actions, err := repo.ListActions("h1")
	if err != nil {
		t.Fatalf("ListActions failed: %v", err)
	}
	if len(actions) != 100 {
		t.Fatalf("expected 100 actions, got %d", len(actions))
	}
}
