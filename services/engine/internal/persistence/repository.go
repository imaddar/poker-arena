package persistence

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

var (
	ErrTableRunNotFound = errors.New("table run not found")
	ErrHandNotFound     = errors.New("hand not found")
	ErrHandAlreadyExists = errors.New("hand already exists")
)

type TableRunStatus string

const (
	TableRunStatusIdle      TableRunStatus = "idle"
	TableRunStatusRunning   TableRunStatus = "running"
	TableRunStatusStopped   TableRunStatus = "stopped"
	TableRunStatusFailed    TableRunStatus = "failed"
	TableRunStatusCompleted TableRunStatus = "completed"
)

type HandRecord struct {
	HandID        string
	TableID       string
	HandNo        uint64
	StartedAt     time.Time
	EndedAt       *time.Time
	FinalPhase    domain.HandPhase
	FinalState    domain.HandState
	WinnerSummary []domain.PotAward
}

type ActionRecord struct {
	HandID      string
	Street      domain.Street
	ActingSeat  domain.SeatNo
	Action      domain.ActionKind
	Amount      *uint32
	IsFallback  bool
	At          time.Time
}

type TableRunRecord struct {
	TableID         string
	Status          TableRunStatus
	StartedAt       time.Time
	EndedAt         *time.Time
	Error           string
	HandsRequested  int
	HandsCompleted  int
	TotalActions    int
	TotalFallbacks  int
	CurrentHandNo   uint64
}

type Repository interface {
	UpsertTableRun(record TableRunRecord) error
	GetTableRun(tableID string) (TableRunRecord, bool, error)
	CreateHand(record HandRecord) error
	CompleteHand(handID string, final HandRecord) error
	AppendAction(record ActionRecord) error
	ListHands(tableID string) ([]HandRecord, error)
	ListActions(handID string) ([]ActionRecord, error)
}

type inMemoryRepository struct {
	mu sync.RWMutex

	tableRuns map[string]TableRunRecord
	hands     map[string]HandRecord
	actions   map[string][]ActionRecord
}

func NewInMemoryRepository() Repository {
	return &inMemoryRepository{
		tableRuns: make(map[string]TableRunRecord),
		hands:     make(map[string]HandRecord),
		actions:   make(map[string][]ActionRecord),
	}
}

func (r *inMemoryRepository) UpsertTableRun(record TableRunRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tableRuns[record.TableID] = cloneTableRunRecord(record)
	return nil
}

func (r *inMemoryRepository) GetTableRun(tableID string) (TableRunRecord, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	record, ok := r.tableRuns[tableID]
	if !ok {
		return TableRunRecord{}, false, nil
	}
	return cloneTableRunRecord(record), true, nil
}

func (r *inMemoryRepository) CreateHand(record HandRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.hands[record.HandID]; exists {
		return ErrHandAlreadyExists
	}
	r.hands[record.HandID] = cloneHandRecord(record)
	return nil
}

func (r *inMemoryRepository) CompleteHand(handID string, final HandRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.hands[handID]; !exists {
		return ErrHandNotFound
	}
	record := cloneHandRecord(final)
	record.HandID = handID
	r.hands[handID] = record
	return nil
}

func (r *inMemoryRepository) AppendAction(record ActionRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions[record.HandID] = append(r.actions[record.HandID], cloneActionRecord(record))
	return nil
}

func (r *inMemoryRepository) ListHands(tableID string) ([]HandRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	hands := make([]HandRecord, 0, len(r.hands))
	for _, record := range r.hands {
		if record.TableID != tableID {
			continue
		}
		hands = append(hands, cloneHandRecord(record))
	}
	sort.Slice(hands, func(i, j int) bool {
		if hands[i].HandNo == hands[j].HandNo {
			return hands[i].HandID < hands[j].HandID
		}
		return hands[i].HandNo < hands[j].HandNo
	})
	return hands, nil
}

func (r *inMemoryRepository) ListActions(handID string) ([]ActionRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	records := r.actions[handID]
	out := make([]ActionRecord, 0, len(records))
	for _, record := range records {
		out = append(out, cloneActionRecord(record))
	}
	return out, nil
}

func cloneTableRunRecord(record TableRunRecord) TableRunRecord {
	out := record
	if record.EndedAt != nil {
		endedAt := *record.EndedAt
		out.EndedAt = &endedAt
	}
	return out
}

func cloneHandRecord(record HandRecord) HandRecord {
	out := record
	out.FinalState = cloneHandState(record.FinalState)
	out.WinnerSummary = clonePotAwards(record.WinnerSummary)
	if record.EndedAt != nil {
		endedAt := *record.EndedAt
		out.EndedAt = &endedAt
	}
	return out
}

func cloneActionRecord(record ActionRecord) ActionRecord {
	out := record
	if record.Amount != nil {
		amount := *record.Amount
		out.Amount = &amount
	}
	return out
}

func clonePotAwards(awards []domain.PotAward) []domain.PotAward {
	if len(awards) == 0 {
		return nil
	}
	out := make([]domain.PotAward, 0, len(awards))
	for _, award := range awards {
		cloned := award
		cloned.Seats = append([]domain.SeatNo(nil), award.Seats...)
		out = append(out, cloned)
	}
	return out
}

func cloneHandState(state domain.HandState) domain.HandState {
	cloned := state
	cloned.Board = append([]domain.Card(nil), state.Board...)
	cloned.Deck = append([]domain.Card(nil), state.Deck...)
	cloned.Seats = append([]domain.SeatState(nil), state.Seats...)
	cloned.ShowdownAwards = clonePotAwards(state.ShowdownAwards)
	if state.LastAggressorSeat != nil {
		seat := *state.LastAggressorSeat
		cloned.LastAggressorSeat = &seat
	}
	if len(state.HoleCards) > 0 {
		cloned.HoleCards = make([]domain.SeatCards, 0, len(state.HoleCards))
		for _, seatCards := range state.HoleCards {
			cloned.HoleCards = append(cloned.HoleCards, domain.SeatCards{
				SeatNo: seatCards.SeatNo,
				Cards:  append([]domain.Card(nil), seatCards.Cards...),
			})
		}
	}
	return cloned
}
