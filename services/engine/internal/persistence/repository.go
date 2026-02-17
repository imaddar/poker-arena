package persistence

import (
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

var (
	ErrTableRunNotFound     = errors.New("table run not found")
	ErrHandNotFound         = errors.New("hand not found")
	ErrHandAlreadyExists    = errors.New("hand already exists")
	ErrUserNotFound         = errors.New("user not found")
	ErrAgentNotFound        = errors.New("agent not found")
	ErrAgentVersionExists   = errors.New("agent version already exists")
	ErrAgentVersionNotFound = errors.New("agent version not found")
	ErrTableNotFound        = errors.New("table not found")
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
	HandID     string
	Street     domain.Street
	ActingSeat domain.SeatNo
	Action     domain.ActionKind
	Amount     *uint32
	IsFallback bool
	At         time.Time
}

type TableRunRecord struct {
	TableID        string
	Status         TableRunStatus
	StartedAt      time.Time
	EndedAt        *time.Time
	Error          string
	HandsRequested int
	HandsCompleted int
	TotalActions   int
	TotalFallbacks int
	CurrentHandNo  uint64
}

type UserRecord struct {
	ID        string
	Name      string
	Token     string
	CreatedAt time.Time
}

type AgentRecord struct {
	ID        string
	UserID    string
	Name      string
	CreatedAt time.Time
}

type AgentVersionRecord struct {
	ID          string
	AgentID     string
	Version     int
	EndpointURL string
	ConfigJSON  json.RawMessage
	CreatedAt   time.Time
}

type TableRecord struct {
	ID         string
	Name       string
	MaxSeats   uint8
	SmallBlind uint32
	BigBlind   uint32
	Status     string
	CreatedAt  time.Time
}

type SeatRecord struct {
	ID             string
	TableID        string
	SeatNo         domain.SeatNo
	AgentID        string
	AgentVersionID string
	Stack          uint32
	Status         domain.SeatStatus
	CreatedAt      time.Time
}

type Repository interface {
	UpsertTableRun(record TableRunRecord) error
	GetTableRun(tableID string) (TableRunRecord, bool, error)
	GetHand(handID string) (HandRecord, bool, error)
	CreateHand(record HandRecord) error
	CompleteHand(handID string, final HandRecord) error
	AppendAction(record ActionRecord) error
	ListHands(tableID string) ([]HandRecord, error)
	ListActions(handID string) ([]ActionRecord, error)
	CreateUser(record UserRecord) error
	CreateAgent(record AgentRecord) error
	CreateAgentVersion(record AgentVersionRecord) error
	CreateTable(record TableRecord) error
	UpsertSeat(record SeatRecord) error
	GetTable(tableID string) (TableRecord, bool, error)
	ListTables() ([]TableRecord, error)
	ListSeats(tableID string) ([]SeatRecord, error)
	GetAgentVersion(versionID string) (AgentVersionRecord, bool, error)
}

type inMemoryRepository struct {
	mu sync.RWMutex

	tableRuns map[string]TableRunRecord
	hands     map[string]HandRecord
	actions   map[string][]ActionRecord
	users     map[string]UserRecord
	agents    map[string]AgentRecord
	versions  map[string]AgentVersionRecord
	tables    map[string]TableRecord
	seats     map[string]map[domain.SeatNo]SeatRecord
}

func NewInMemoryRepository() Repository {
	return &inMemoryRepository{
		tableRuns: make(map[string]TableRunRecord),
		hands:     make(map[string]HandRecord),
		actions:   make(map[string][]ActionRecord),
		users:     make(map[string]UserRecord),
		agents:    make(map[string]AgentRecord),
		versions:  make(map[string]AgentVersionRecord),
		tables:    make(map[string]TableRecord),
		seats:     make(map[string]map[domain.SeatNo]SeatRecord),
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

func (r *inMemoryRepository) GetHand(handID string) (HandRecord, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	record, ok := r.hands[handID]
	if !ok {
		return HandRecord{}, false, nil
	}
	return cloneHandRecord(record), true, nil
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
	if _, exists := r.hands[record.HandID]; !exists {
		return ErrHandNotFound
	}
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

func (r *inMemoryRepository) CreateUser(record UserRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users[record.ID] = cloneUserRecord(record)
	return nil
}

func (r *inMemoryRepository) CreateAgent(record AgentRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.users[record.UserID]; !exists {
		return ErrUserNotFound
	}
	r.agents[record.ID] = cloneAgentRecord(record)
	return nil
}

func (r *inMemoryRepository) CreateAgentVersion(record AgentVersionRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.agents[record.AgentID]; !exists {
		return ErrAgentNotFound
	}
	for _, version := range r.versions {
		if version.AgentID == record.AgentID && version.Version == record.Version {
			return ErrAgentVersionExists
		}
	}
	r.versions[record.ID] = cloneAgentVersionRecord(record)
	return nil
}

func (r *inMemoryRepository) CreateTable(record TableRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tables[record.ID] = cloneTableRecord(record)
	return nil
}

func (r *inMemoryRepository) UpsertSeat(record SeatRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tables[record.TableID]; !exists {
		return ErrTableNotFound
	}
	if _, exists := r.agents[record.AgentID]; !exists {
		return ErrAgentNotFound
	}
	version, exists := r.versions[record.AgentVersionID]
	if !exists {
		return ErrAgentVersionNotFound
	}
	if version.AgentID != record.AgentID {
		return ErrAgentVersionNotFound
	}
	if _, exists := r.seats[record.TableID]; !exists {
		r.seats[record.TableID] = make(map[domain.SeatNo]SeatRecord)
	}
	r.seats[record.TableID][record.SeatNo] = cloneSeatRecord(record)
	return nil
}

func (r *inMemoryRepository) GetTable(tableID string) (TableRecord, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	record, ok := r.tables[tableID]
	if !ok {
		return TableRecord{}, false, nil
	}
	return cloneTableRecord(record), true, nil
}

func (r *inMemoryRepository) ListTables() ([]TableRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]TableRecord, 0, len(r.tables))
	for _, record := range r.tables {
		out = append(out, cloneTableRecord(record))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (r *inMemoryRepository) ListSeats(tableID string) ([]SeatRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tableSeats, ok := r.seats[tableID]
	if !ok {
		return []SeatRecord{}, nil
	}
	out := make([]SeatRecord, 0, len(tableSeats))
	for _, seat := range tableSeats {
		out = append(out, cloneSeatRecord(seat))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].SeatNo < out[j].SeatNo
	})
	return out, nil
}

func (r *inMemoryRepository) GetAgentVersion(versionID string) (AgentVersionRecord, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	record, ok := r.versions[versionID]
	if !ok {
		return AgentVersionRecord{}, false, nil
	}
	return cloneAgentVersionRecord(record), true, nil
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

func cloneUserRecord(record UserRecord) UserRecord {
	return record
}

func cloneAgentRecord(record AgentRecord) AgentRecord {
	return record
}

func cloneAgentVersionRecord(record AgentVersionRecord) AgentVersionRecord {
	out := record
	out.ConfigJSON = append([]byte(nil), record.ConfigJSON...)
	return out
}

func cloneTableRecord(record TableRecord) TableRecord {
	return record
}

func cloneSeatRecord(record SeatRecord) SeatRecord {
	return record
}
