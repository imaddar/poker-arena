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

func TestGetHands_ReturnsPersistedHandsForTable(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	now := time.Now().UTC()
	if err := repo.UpsertTableRun(persistence.TableRunRecord{
		TableID:   "table-1",
		Status:    persistence.TableRunStatusCompleted,
		StartedAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("UpsertTableRun failed: %v", err)
	}
	if err := repo.CreateHand(persistence.HandRecord{
		HandID:     "hand-2",
		TableID:    "table-1",
		HandNo:     2,
		StartedAt:  now.Add(-30 * time.Second),
		FinalPhase: domain.HandPhaseComplete,
	}); err != nil {
		t.Fatalf("CreateHand hand-2 failed: %v", err)
	}
	if err := repo.CreateHand(persistence.HandRecord{
		HandID:     "hand-1",
		TableID:    "table-1",
		HandNo:     1,
		StartedAt:  now.Add(-45 * time.Second),
		FinalPhase: domain.HandPhaseComplete,
	}); err != nil {
		t.Fatalf("CreateHand hand-1 failed: %v", err)
	}
	if err := repo.CreateHand(persistence.HandRecord{
		HandID:     "hand-other",
		TableID:    "table-2",
		HandNo:     1,
		StartedAt:  now.Add(-45 * time.Second),
		FinalPhase: domain.HandPhaseComplete,
	}); err != nil {
		t.Fatalf("CreateHand hand-other failed: %v", err)
	}

	server := NewServer(repo, nil, nil, ServerConfig{})
	req := httptest.NewRequest(http.MethodGet, "/tables/table-1/hands", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	var hands []handResponse
	if err := json.Unmarshal(w.Body.Bytes(), &hands); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(hands) != 2 {
		t.Fatalf("expected 2 hands, got %d", len(hands))
	}
	if hands[0].HandID != "hand-1" || hands[1].HandID != "hand-2" {
		t.Fatalf("expected sorted hand IDs [hand-1 hand-2], got [%s %s]", hands[0].HandID, hands[1].HandID)
	}
}

func TestListTables_AdminCanListTables(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	now := time.Now().UTC()
	if err := repo.CreateTable(persistence.TableRecord{
		ID:         "table-2",
		Name:       "Beta",
		MaxSeats:   6,
		SmallBlind: 100,
		BigBlind:   200,
		Status:     string(persistence.TableRunStatusIdle),
		CreatedAt:  now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateTable table-2 failed: %v", err)
	}
	if err := repo.CreateTable(persistence.TableRecord{
		ID:         "table-1",
		Name:       "Alpha",
		MaxSeats:   6,
		SmallBlind: 50,
		BigBlind:   100,
		Status:     string(persistence.TableRunStatusIdle),
		CreatedAt:  now.Add(1 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateTable table-1 failed: %v", err)
	}

	server := NewServer(repo, nil, nil, ServerConfig{AdminBearerTokens: map[string]struct{}{"admin": {}}})
	req := httptest.NewRequest(http.MethodGet, "/tables", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	var tables []persistence.TableRecord
	if err := json.Unmarshal(w.Body.Bytes(), &tables); err != nil {
		t.Fatalf("failed to decode tables response: %v", err)
	}
	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}
	if tables[0].ID != "table-1" || tables[1].ID != "table-2" {
		t.Fatalf("expected sorted table IDs [table-1 table-2], got [%s %s]", tables[0].ID, tables[1].ID)
	}
}

func TestListTables_UnauthenticatedReturnsUnauthorized(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(repo, nil, nil, ServerConfig{AdminBearerTokens: map[string]struct{}{"admin": {}}})

	req := httptest.NewRequest(http.MethodGet, "/tables", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusUnauthorized, w.Code, w.Body.String())
	}
}

func TestListTables_SeatTokenReturnsForbidden(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(repo, nil, nil, ServerConfig{
		AdminBearerTokens: map[string]struct{}{"admin": {}},
		SeatBearerTokens:  map[string]domain.SeatNo{"seat1": 1},
	})

	req := httptest.NewRequest(http.MethodGet, "/tables", nil)
	req.Header.Set("Authorization", "Bearer seat1")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusForbidden, w.Code, w.Body.String())
	}
}

func TestCreateUser_Succeeds(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(repo, nil, nil, ServerConfig{AdminBearerTokens: map[string]struct{}{"admin": {}}})
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"operator","token":"operator-token"}`))
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}
	var user persistence.UserRecord
	if err := json.Unmarshal(w.Body.Bytes(), &user); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if user.Name != "operator" || user.Token != "operator-token" || user.ID == "" {
		t.Fatalf("unexpected user response: %+v", user)
	}
}

func TestCreateAgent_MissingUserFails(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(repo, nil, nil, ServerConfig{AdminBearerTokens: map[string]struct{}{"admin": {}}})
	req := httptest.NewRequest(http.MethodPost, "/agents", strings.NewReader(`{"user_id":"missing","name":"agent-a"}`))
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d got %d body=%s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestCreateAgentVersion_ValidatesEndpointAndAllowlist(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	now := time.Now().UTC()
	if err := repo.CreateUser(persistence.UserRecord{ID: "u1", Name: "u", Token: "tok", CreatedAt: now}); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if err := repo.CreateAgent(persistence.AgentRecord{ID: "a1", UserID: "u1", Name: "a", CreatedAt: now}); err != nil {
		t.Fatalf("CreateAgent failed: %v", err)
	}
	server := NewServer(repo, nil, nil, ServerConfig{
		AdminBearerTokens: map[string]struct{}{"admin": {}},
		AllowedAgentHosts: map[string]struct{}{"agent.local:9001": {}},
	})

	req := httptest.NewRequest(http.MethodPost, "/agents/a1/versions", strings.NewReader(`{"endpoint_url":"http://blocked.local:9001/cb"}`))
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d got %d body=%s", http.StatusBadRequest, w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/agents/a1/versions", strings.NewReader(`{"endpoint_url":"http://agent.local:9001/cb"}`))
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestCreateTableJoinAndState(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	now := time.Now().UTC()
	if err := repo.CreateUser(persistence.UserRecord{ID: "u1", Name: "u", Token: "tok", CreatedAt: now}); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if err := repo.CreateAgent(persistence.AgentRecord{ID: "a1", UserID: "u1", Name: "a1", CreatedAt: now}); err != nil {
		t.Fatalf("CreateAgent failed: %v", err)
	}
	if err := repo.CreateAgentVersion(persistence.AgentVersionRecord{ID: "v1", AgentID: "a1", Version: 1, EndpointURL: "http://agent.local:9001/cb", CreatedAt: now}); err != nil {
		t.Fatalf("CreateAgentVersion failed: %v", err)
	}

	server := NewServer(repo, nil, nil, ServerConfig{AdminBearerTokens: map[string]struct{}{"admin": {}}})
	req := httptest.NewRequest(http.MethodPost, "/tables", strings.NewReader(`{"name":"t1","max_seats":6,"small_blind":50,"big_blind":100}`))
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create table failed: %d body=%s", w.Code, w.Body.String())
	}
	var table persistence.TableRecord
	if err := json.Unmarshal(w.Body.Bytes(), &table); err != nil {
		t.Fatalf("decode table failed: %v", err)
	}

	req = httptest.NewRequest(http.MethodPost, "/tables/"+table.ID+"/join", strings.NewReader(`{"seat_no":1,"agent_id":"a1","agent_version_id":"v1","stack":10000,"status":"active"}`))
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("join failed: %d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/tables/"+table.ID+"/state", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("state failed: %d body=%s", w.Code, w.Body.String())
	}
	var state tableStateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode state failed: %v", err)
	}
	if state.Table.ID != table.ID || len(state.Seats) != 1 || state.Seats[0].SeatNo != 1 {
		t.Fatalf("unexpected state payload: %+v", state)
	}
}

func TestStart_WithPersistedSeatsOnly_Succeeds(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	now := time.Now().UTC()
	if err := repo.CreateUser(persistence.UserRecord{ID: "u1", Name: "u", Token: "tok", CreatedAt: now}); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if err := repo.CreateAgent(persistence.AgentRecord{ID: "a1", UserID: "u1", Name: "a1", CreatedAt: now}); err != nil {
		t.Fatalf("CreateAgent failed: %v", err)
	}
	if err := repo.CreateAgent(persistence.AgentRecord{ID: "a2", UserID: "u1", Name: "a2", CreatedAt: now}); err != nil {
		t.Fatalf("CreateAgent failed: %v", err)
	}
	if err := repo.CreateAgentVersion(persistence.AgentVersionRecord{ID: "v1", AgentID: "a1", Version: 1, EndpointURL: "http://agent.local:9001/cb", CreatedAt: now}); err != nil {
		t.Fatalf("CreateAgentVersion failed: %v", err)
	}
	if err := repo.CreateAgentVersion(persistence.AgentVersionRecord{ID: "v2", AgentID: "a2", Version: 1, EndpointURL: "http://agent.local:9002/cb", CreatedAt: now}); err != nil {
		t.Fatalf("CreateAgentVersion failed: %v", err)
	}
	if err := repo.CreateTable(persistence.TableRecord{ID: "table-1", Name: "t", MaxSeats: 6, SmallBlind: 50, BigBlind: 100, Status: "idle", CreatedAt: now}); err != nil {
		t.Fatalf("CreateTable failed: %v", err)
	}
	if err := repo.UpsertSeat(persistence.SeatRecord{ID: "s1", TableID: "table-1", SeatNo: 1, AgentID: "a1", AgentVersionID: "v1", Stack: 10000, Status: domain.SeatStatusActive, CreatedAt: now}); err != nil {
		t.Fatalf("UpsertSeat s1 failed: %v", err)
	}
	if err := repo.UpsertSeat(persistence.SeatRecord{ID: "s2", TableID: "table-1", SeatNo: 2, AgentID: "a2", AgentVersionID: "v2", Stack: 10000, Status: domain.SeatStatusActive, CreatedAt: now}); err != nil {
		t.Fatalf("UpsertSeat s2 failed: %v", err)
	}

	server := NewServer(
		repo,
		func(_ tablerunner.ActionProvider, cfg tablerunner.RunnerConfig) Runner { return fakeRunner{cfg: cfg} },
		func(_ string, start StartRequest, _ ServerConfig) (tablerunner.ActionProvider, error) {
			if len(start.Seats) != 2 {
				return nil, fmt.Errorf("expected 2 hydrated seats, got %d", len(start.Seats))
			}
			return fakeProvider{}, nil
		},
		ServerConfig{
			AdminBearerTokens: map[string]struct{}{"admin": {}},
			AllowedAgentHosts: map[string]struct{}{"agent.local:9001": {}, "agent.local:9002": {}},
		},
	)

	req := httptest.NewRequest(http.MethodPost, "/tables/table-1/start", strings.NewReader(`{"hands_to_run":1}`))
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestStart_WithPersistedTableMissingReturnsNotFound(t *testing.T) {
	t.Parallel()
	repo := persistence.NewInMemoryRepository()
	server := NewServer(
		repo,
		func(_ tablerunner.ActionProvider, cfg tablerunner.RunnerConfig) Runner { return fakeRunner{cfg: cfg} },
		func(_ string, _ StartRequest, _ ServerConfig) (tablerunner.ActionProvider, error) {
			return fakeProvider{}, nil
		},
		ServerConfig{AdminBearerTokens: map[string]struct{}{"admin": {}}},
	)
	req := httptest.NewRequest(http.MethodPost, "/tables/missing/start", strings.NewReader(`{"hands_to_run":1}`))
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected %d got %d body=%s", http.StatusNotFound, w.Code, w.Body.String())
	}
}

func TestResourceRoutes_RequireAdminAuth(t *testing.T) {
	t.Parallel()
	repo := persistence.NewInMemoryRepository()
	server := NewServer(repo, nil, nil, ServerConfig{AdminBearerTokens: map[string]struct{}{"admin": {}}})
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"n","token":"t"}`))
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d got %d body=%s", http.StatusUnauthorized, w.Code, w.Body.String())
	}
}

func TestGetHands_UnknownTableReturnsNotFound(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(repo, nil, nil, ServerConfig{})
	req := httptest.NewRequest(http.MethodGet, "/tables/missing/hands", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusNotFound, w.Code, w.Body.String())
	}
}

func TestGetActions_ReturnsPersistedActionsForHand(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	now := time.Now().UTC()
	for _, hand := range []persistence.HandRecord{
		{HandID: "hand-1", TableID: "table-1", HandNo: 1, StartedAt: now.Add(-2 * time.Minute)},
		{HandID: "hand-2", TableID: "table-1", HandNo: 2, StartedAt: now.Add(-1 * time.Minute)},
	} {
		if err := repo.CreateHand(hand); err != nil {
			t.Fatalf("CreateHand failed: %v", err)
		}
	}
	if err := repo.AppendAction(persistence.ActionRecord{
		HandID:     "hand-1",
		Street:     domain.StreetPreflop,
		ActingSeat: 1,
		Action:     domain.ActionCall,
		At:         now,
	}); err != nil {
		t.Fatalf("AppendAction failed: %v", err)
	}
	if err := repo.AppendAction(persistence.ActionRecord{
		HandID:     "hand-1",
		Street:     domain.StreetFlop,
		ActingSeat: 2,
		Action:     domain.ActionCheck,
		At:         now.Add(1 * time.Second),
	}); err != nil {
		t.Fatalf("AppendAction failed: %v", err)
	}
	if err := repo.AppendAction(persistence.ActionRecord{
		HandID:     "hand-2",
		Street:     domain.StreetPreflop,
		ActingSeat: 1,
		Action:     domain.ActionFold,
		At:         now,
	}); err != nil {
		t.Fatalf("AppendAction failed: %v", err)
	}

	server := NewServer(repo, nil, nil, ServerConfig{})
	req := httptest.NewRequest(http.MethodGet, "/hands/hand-1/actions", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	var actions []actionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &actions); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}
	if actions[0].Action != domain.ActionCall || actions[1].Action != domain.ActionCheck {
		t.Fatalf("unexpected action sequence: %+v", actions)
	}
}

func TestGetReplay_ReturnsHandAndOrderedActions(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	now := time.Now().UTC()
	if err := repo.UpsertTableRun(persistence.TableRunRecord{
		TableID:        "table-1",
		Status:         persistence.TableRunStatusCompleted,
		StartedAt:      now.Add(-2 * time.Minute),
		HandsRequested: 1,
		HandsCompleted: 1,
		CurrentHandNo:  1,
	}); err != nil {
		t.Fatalf("UpsertTableRun failed: %v", err)
	}
	if err := repo.CreateHand(persistence.HandRecord{
		HandID:     "hand-1",
		TableID:    "table-1",
		HandNo:     1,
		StartedAt:  now.Add(-time.Minute),
		FinalPhase: domain.HandPhaseComplete,
		FinalState: domain.HandState{
			HandID: "hand-1", TableID: "table-1", HandNo: 1, Phase: domain.HandPhaseComplete,
			Seats: []domain.SeatState{
				{SeatNo: 1, Folded: false, Status: domain.SeatStatusActive},
				{SeatNo: 2, Folded: true, Status: domain.SeatStatusActive},
			},
			HoleCards: []domain.SeatCards{
				{SeatNo: 1, Cards: []domain.Card{{Rank: 14, Suit: domain.SuitSpades}, {Rank: 13, Suit: domain.SuitSpades}}},
				{SeatNo: 2, Cards: []domain.Card{{Rank: 2, Suit: domain.SuitClubs}, {Rank: 7, Suit: domain.SuitDiamonds}}},
			},
		},
		WinnerSummary: []domain.PotAward{{Amount: 100, Seats: []domain.SeatNo{1}, Reason: "showdown"}},
	}); err != nil {
		t.Fatalf("CreateHand failed: %v", err)
	}
	if err := repo.AppendAction(persistence.ActionRecord{
		HandID:     "hand-1",
		Street:     domain.StreetPreflop,
		ActingSeat: 1,
		Action:     domain.ActionCall,
		At:         now,
	}); err != nil {
		t.Fatalf("AppendAction #1 failed: %v", err)
	}
	if err := repo.AppendAction(persistence.ActionRecord{
		HandID:     "hand-1",
		Street:     domain.StreetFlop,
		ActingSeat: 2,
		Action:     domain.ActionCheck,
		At:         now.Add(time.Second),
	}); err != nil {
		t.Fatalf("AppendAction #2 failed: %v", err)
	}

	server := NewServer(repo, nil, nil, ServerConfig{})
	req := httptest.NewRequest(http.MethodGet, "/hands/hand-1/replay", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	var replay handReplayResponse
	if err := json.Unmarshal(w.Body.Bytes(), &replay); err != nil {
		t.Fatalf("decode replay failed: %v", err)
	}
	if replay.HandID != "hand-1" || replay.TableID != "table-1" || replay.HandNo != 1 {
		t.Fatalf("unexpected replay metadata: %+v", replay)
	}
	if len(replay.WinnerSummary) != 1 || replay.WinnerSummary[0].Amount != 100 {
		t.Fatalf("unexpected winner summary: %+v", replay.WinnerSummary)
	}
	if replay.FinalState.HandID != "hand-1" {
		t.Fatalf("expected final state hand id hand-1, got %q", replay.FinalState.HandID)
	}
	if len(replay.Actions) != 2 {
		t.Fatalf("expected 2 replay actions, got %d", len(replay.Actions))
	}
	if replay.Actions[0].Action != domain.ActionCall || replay.Actions[1].Action != domain.ActionCheck {
		t.Fatalf("expected ordered actions [call,check], got [%s,%s]", replay.Actions[0].Action, replay.Actions[1].Action)
	}
	if replay.Analytics.TotalActions != 2 {
		t.Fatalf("expected analytics total_actions=2, got %d", replay.Analytics.TotalActions)
	}
	if replay.Analytics.FallbackActions != 0 {
		t.Fatalf("expected analytics fallback_actions=0, got %d", replay.Analytics.FallbackActions)
	}
	if replay.Analytics.ActionsByStreet[domain.StreetPreflop] != 1 || replay.Analytics.ActionsByStreet[domain.StreetFlop] != 1 {
		t.Fatalf("unexpected actions_by_street: %+v", replay.Analytics.ActionsByStreet)
	}
	if replay.Analytics.ActionsBySeat[1] != 1 || replay.Analytics.ActionsBySeat[2] != 1 {
		t.Fatalf("unexpected actions_by_seat: %+v", replay.Analytics.ActionsBySeat)
	}
}

func TestGetReplay_RedactHoleCardsRedactsOnlyFoldedSeats(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	now := time.Now().UTC()
	if err := repo.UpsertTableRun(persistence.TableRunRecord{
		TableID:   "table-1",
		Status:    persistence.TableRunStatusCompleted,
		StartedAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("UpsertTableRun failed: %v", err)
	}
	if err := repo.CreateHand(persistence.HandRecord{
		HandID:     "hand-1",
		TableID:    "table-1",
		HandNo:     1,
		StartedAt:  now.Add(-30 * time.Second),
		FinalPhase: domain.HandPhaseComplete,
		FinalState: domain.HandState{
			HandID: "hand-1", TableID: "table-1", HandNo: 1, Phase: domain.HandPhaseComplete,
			Seats: []domain.SeatState{
				{SeatNo: 1, Folded: false, Status: domain.SeatStatusActive},
				{SeatNo: 2, Folded: true, Status: domain.SeatStatusActive},
			},
			HoleCards: []domain.SeatCards{
				{SeatNo: 1, Cards: []domain.Card{{Rank: 14, Suit: domain.SuitSpades}, {Rank: 10, Suit: domain.SuitSpades}}},
				{SeatNo: 2, Cards: []domain.Card{{Rank: 4, Suit: domain.SuitClubs}, {Rank: 8, Suit: domain.SuitClubs}}},
			},
		},
	}); err != nil {
		t.Fatalf("CreateHand failed: %v", err)
	}

	server := NewServer(repo, nil, nil, ServerConfig{})
	req := httptest.NewRequest(http.MethodGet, "/hands/hand-1/replay?redact_hole_cards=true", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	var replay handReplayResponse
	if err := json.Unmarshal(w.Body.Bytes(), &replay); err != nil {
		t.Fatalf("decode replay failed: %v", err)
	}
	if len(replay.FinalState.HoleCards) != 2 {
		t.Fatalf("expected 2 seat hole-card sets, got %d", len(replay.FinalState.HoleCards))
	}
	if len(replay.FinalState.HoleCards[0].Cards) != 2 {
		t.Fatalf("expected non-folded seat cards to remain visible, got %d cards", len(replay.FinalState.HoleCards[0].Cards))
	}
	if len(replay.FinalState.HoleCards[1].Cards) != 0 {
		t.Fatalf("expected folded seat cards to be redacted, got %d cards", len(replay.FinalState.HoleCards[1].Cards))
	}
}

func TestGetReplay_InvalidRedactHoleCardsValueReturnsBadRequest(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(repo, nil, nil, ServerConfig{})
	req := httptest.NewRequest(http.MethodGet, "/hands/hand-1/replay?redact_hole_cards=maybe", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestAuth_ValidSeatTokenCanAccessHistoryRoute(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	now := time.Now().UTC()
	if err := repo.UpsertTableRun(persistence.TableRunRecord{
		TableID:   "table-1",
		Status:    persistence.TableRunStatusCompleted,
		StartedAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("UpsertTableRun failed: %v", err)
	}
	if err := repo.CreateHand(persistence.HandRecord{
		HandID:    "hand-1",
		TableID:   "table-1",
		HandNo:    1,
		StartedAt: now.Add(-30 * time.Second),
		FinalState: domain.HandState{
			HandID: "hand-1",
			Seats: []domain.SeatState{
				{SeatNo: 1}, {SeatNo: 2},
			},
		},
	}); err != nil {
		t.Fatalf("CreateHand failed: %v", err)
	}

	server := NewServer(repo, nil, nil, ServerConfig{
		AdminBearerTokens: map[string]struct{}{"admin": {}},
		SeatBearerTokens:  map[string]domain.SeatNo{"seat1": 1},
	})
	req := httptest.NewRequest(http.MethodGet, "/hands/hand-1/replay", nil)
	req.Header.Set("Authorization", "Bearer seat1")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestAuth_SeatTokenForbiddenOnControlRoutes(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(repo, nil, nil, ServerConfig{
		AdminBearerTokens: map[string]struct{}{"admin": {}},
		SeatBearerTokens:  map[string]domain.SeatNo{"seat1": 1},
	})

	for _, route := range []string{"/tables/t1/start", "/tables/t1/stop", "/tables/t1/status"} {
		method := http.MethodGet
		if strings.HasSuffix(route, "/start") || strings.HasSuffix(route, "/stop") {
			method = http.MethodPost
		}
		req := httptest.NewRequest(method, route, strings.NewReader(`{"hands_to_run":1,"seats":[{"seat_no":1,"stack":10000,"agent_endpoint":"http://agent.local"}]}`))
		req.Header.Set("Authorization", "Bearer seat1")
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected status %d for %s, got %d body=%s", http.StatusForbidden, route, w.Code, w.Body.String())
		}
	}
}

func TestReplay_SeatTokenNonShowdownRedactsOpponents(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	now := time.Now().UTC()
	if err := repo.UpsertTableRun(persistence.TableRunRecord{
		TableID:   "table-1",
		Status:    persistence.TableRunStatusCompleted,
		StartedAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("UpsertTableRun failed: %v", err)
	}
	if err := repo.CreateHand(persistence.HandRecord{
		HandID:     "hand-1",
		TableID:    "table-1",
		HandNo:     1,
		StartedAt:  now.Add(-30 * time.Second),
		FinalPhase: domain.HandPhaseComplete,
		FinalState: domain.HandState{
			HandID: "hand-1", TableID: "table-1", HandNo: 1, Phase: domain.HandPhaseComplete,
			Seats: []domain.SeatState{
				{SeatNo: 1, Status: domain.SeatStatusActive},
				{SeatNo: 2, Status: domain.SeatStatusActive},
			},
			HoleCards: []domain.SeatCards{
				{SeatNo: 1, Cards: []domain.Card{{Rank: 14, Suit: domain.SuitSpades}, {Rank: 10, Suit: domain.SuitSpades}}},
				{SeatNo: 2, Cards: []domain.Card{{Rank: 4, Suit: domain.SuitClubs}, {Rank: 8, Suit: domain.SuitClubs}}},
			},
		},
		WinnerSummary: []domain.PotAward{{Amount: 100, Seats: []domain.SeatNo{1}, Reason: "uncontested"}},
	}); err != nil {
		t.Fatalf("CreateHand failed: %v", err)
	}
	server := NewServer(repo, nil, nil, ServerConfig{
		AdminBearerTokens: map[string]struct{}{"admin": {}},
		SeatBearerTokens:  map[string]domain.SeatNo{"seat1": 1},
	})

	req := httptest.NewRequest(http.MethodGet, "/hands/hand-1/replay", nil)
	req.Header.Set("Authorization", "Bearer seat1")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	var replay handReplayResponse
	if err := json.Unmarshal(w.Body.Bytes(), &replay); err != nil {
		t.Fatalf("decode replay failed: %v", err)
	}
	if len(replay.FinalState.HoleCards[0].Cards) != 2 {
		t.Fatalf("expected own cards visible, got %d", len(replay.FinalState.HoleCards[0].Cards))
	}
	if len(replay.FinalState.HoleCards[1].Cards) != 0 {
		t.Fatalf("expected opponent cards redacted, got %d", len(replay.FinalState.HoleCards[1].Cards))
	}
}

func TestReplay_SeatTokenShowdownRevealsOpponents(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	now := time.Now().UTC()
	if err := repo.UpsertTableRun(persistence.TableRunRecord{
		TableID:   "table-1",
		Status:    persistence.TableRunStatusCompleted,
		StartedAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("UpsertTableRun failed: %v", err)
	}
	if err := repo.CreateHand(persistence.HandRecord{
		HandID:    "hand-1",
		TableID:   "table-1",
		HandNo:    1,
		StartedAt: now.Add(-30 * time.Second),
		FinalState: domain.HandState{
			HandID: "hand-1",
			Seats: []domain.SeatState{
				{SeatNo: 1}, {SeatNo: 2},
			},
			ShowdownAwards: []domain.PotAward{{Amount: 200, Seats: []domain.SeatNo{1}, Reason: "showdown"}},
			HoleCards: []domain.SeatCards{
				{SeatNo: 1, Cards: []domain.Card{{Rank: 14, Suit: domain.SuitSpades}, {Rank: 13, Suit: domain.SuitSpades}}},
				{SeatNo: 2, Cards: []domain.Card{{Rank: 2, Suit: domain.SuitClubs}, {Rank: 7, Suit: domain.SuitDiamonds}}},
			},
		},
		WinnerSummary: []domain.PotAward{{Amount: 200, Seats: []domain.SeatNo{1}, Reason: "showdown"}},
	}); err != nil {
		t.Fatalf("CreateHand failed: %v", err)
	}
	server := NewServer(repo, nil, nil, ServerConfig{
		AdminBearerTokens: map[string]struct{}{"admin": {}},
		SeatBearerTokens:  map[string]domain.SeatNo{"seat1": 1},
	})

	req := httptest.NewRequest(http.MethodGet, "/hands/hand-1/replay", nil)
	req.Header.Set("Authorization", "Bearer seat1")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}
	var replay handReplayResponse
	if err := json.Unmarshal(w.Body.Bytes(), &replay); err != nil {
		t.Fatalf("decode replay failed: %v", err)
	}
	if len(replay.FinalState.HoleCards[1].Cards) != 2 {
		t.Fatalf("expected showdown opponent cards visible, got %d", len(replay.FinalState.HoleCards[1].Cards))
	}
}

func TestReplay_SeatTokenForbiddenWhenSeatNotInHand(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	now := time.Now().UTC()
	if err := repo.UpsertTableRun(persistence.TableRunRecord{
		TableID:   "table-1",
		Status:    persistence.TableRunStatusCompleted,
		StartedAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("UpsertTableRun failed: %v", err)
	}
	if err := repo.CreateHand(persistence.HandRecord{
		HandID:    "hand-1",
		TableID:   "table-1",
		HandNo:    1,
		StartedAt: now,
		FinalState: domain.HandState{
			HandID: "hand-1",
			Seats: []domain.SeatState{
				{SeatNo: 2},
				{SeatNo: 3},
			},
		},
	}); err != nil {
		t.Fatalf("CreateHand failed: %v", err)
	}
	server := NewServer(repo, nil, nil, ServerConfig{
		AdminBearerTokens: map[string]struct{}{"admin": {}},
		SeatBearerTokens:  map[string]domain.SeatNo{"seat1": 1},
	})

	req := httptest.NewRequest(http.MethodGet, "/hands/hand-1/replay", nil)
	req.Header.Set("Authorization", "Bearer seat1")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusForbidden, w.Code, w.Body.String())
	}
}

func TestActions_SeatTokenForbiddenWhenSeatNotInHand(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	now := time.Now().UTC()
	if err := repo.UpsertTableRun(persistence.TableRunRecord{
		TableID:   "table-1",
		Status:    persistence.TableRunStatusCompleted,
		StartedAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("UpsertTableRun failed: %v", err)
	}
	if err := repo.CreateHand(persistence.HandRecord{
		HandID:    "hand-1",
		TableID:   "table-1",
		HandNo:    1,
		StartedAt: now,
		FinalState: domain.HandState{
			HandID: "hand-1",
			Seats: []domain.SeatState{
				{SeatNo: 2},
				{SeatNo: 3},
			},
		},
	}); err != nil {
		t.Fatalf("CreateHand failed: %v", err)
	}
	server := NewServer(repo, nil, nil, ServerConfig{
		AdminBearerTokens: map[string]struct{}{"admin": {}},
		SeatBearerTokens:  map[string]domain.SeatNo{"seat1": 1},
	})

	req := httptest.NewRequest(http.MethodGet, "/hands/hand-1/actions", nil)
	req.Header.Set("Authorization", "Bearer seat1")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusForbidden, w.Code, w.Body.String())
	}
}

func TestHands_SeatTokenReturnsOnlyParticipatingHands(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	now := time.Now().UTC()
	if err := repo.UpsertTableRun(persistence.TableRunRecord{
		TableID:   "table-1",
		Status:    persistence.TableRunStatusCompleted,
		StartedAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("UpsertTableRun failed: %v", err)
	}
	for _, hand := range []persistence.HandRecord{
		{
			HandID: "hand-1", TableID: "table-1", HandNo: 1, StartedAt: now.Add(-40 * time.Second),
			FinalState: domain.HandState{HandID: "hand-1", Seats: []domain.SeatState{{SeatNo: 1}, {SeatNo: 2}}},
		},
		{
			HandID: "hand-2", TableID: "table-1", HandNo: 2, StartedAt: now.Add(-30 * time.Second),
			FinalState: domain.HandState{HandID: "hand-2", Seats: []domain.SeatState{{SeatNo: 3}, {SeatNo: 4}}},
		},
	} {
		if err := repo.CreateHand(hand); err != nil {
			t.Fatalf("CreateHand failed: %v", err)
		}
	}

	server := NewServer(repo, nil, nil, ServerConfig{
		AdminBearerTokens: map[string]struct{}{"admin": {}},
		SeatBearerTokens:  map[string]domain.SeatNo{"seat1": 1},
	})
	req := httptest.NewRequest(http.MethodGet, "/tables/table-1/hands", nil)
	req.Header.Set("Authorization", "Bearer seat1")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}
	var hands []handResponse
	if err := json.Unmarshal(w.Body.Bytes(), &hands); err != nil {
		t.Fatalf("decode hands failed: %v", err)
	}
	if len(hands) != 1 || hands[0].HandID != "hand-1" {
		t.Fatalf("expected only hand-1 for seat token, got %+v", hands)
	}
}

func TestGetReplay_UnknownHandReturnsNotFound(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(repo, nil, nil, ServerConfig{})
	req := httptest.NewRequest(http.MethodGet, "/hands/missing/replay", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusNotFound, w.Code, w.Body.String())
	}
}

func TestGetReplay_RequiresAuth(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(repo, nil, nil, ServerConfig{AdminBearerTokens: map[string]struct{}{"secret": {}}})
	req := httptest.NewRequest(http.MethodGet, "/hands/hand-1/replay", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusUnauthorized, w.Code, w.Body.String())
	}
}

func TestStartRunPersistsHandStartedAtIndependentlyFromRunStartedAt(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(
		repo,
		func(_ tablerunner.ActionProvider, cfg tablerunner.RunnerConfig) Runner {
			return fakeRunner{cfg: cfg}
		},
		func(tableID string, _ StartRequest, _ ServerConfig) (tablerunner.ActionProvider, error) {
			return fakeProvider{}, nil
		},
		ServerConfig{
			AdminBearerTokens: map[string]struct{}{"test-token": {}},
			AllowedAgentHosts: map[string]struct{}{"agent.local:9001": {}, "agent.local:9002": {}},
		},
	)

	req := httptest.NewRequest(http.MethodPost, "/tables/table-1/start", strings.NewReader(`{
		"hands_to_run": 1,
		"seats": [
			{"seat_no": 1, "stack": 10000, "status": "active", "agent_endpoint": "http://agent.local:9001/callback"},
			{"seat_no": 2, "stack": 10000, "status": "active", "agent_endpoint": "http://agent.local:9002/callback"}
		]
	}`))
	req.Header.Set("Authorization", "Bearer test-token")
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

func TestAuth_MissingBearerTokenReturnsUnauthorized(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(repo, nil, nil, ServerConfig{AdminBearerTokens: map[string]struct{}{"secret": {}}})
	req := httptest.NewRequest(http.MethodGet, "/tables/table-1/hands", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusUnauthorized, w.Code, w.Body.String())
	}
}

func TestAuth_WrongBearerTokenReturnsUnauthorized(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(repo, nil, nil, ServerConfig{AdminBearerTokens: map[string]struct{}{"secret": {}}})
	req := httptest.NewRequest(http.MethodGet, "/tables/table-1/hands", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusUnauthorized, w.Code, w.Body.String())
	}
}

func TestAuth_CorrectBearerTokenAllowsRequest(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(repo, nil, nil, ServerConfig{AdminBearerTokens: map[string]struct{}{"secret": {}}})
	req := httptest.NewRequest(http.MethodGet, "/tables/table-1/hands", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusNotFound, w.Code, w.Body.String())
	}
}

func TestStartRejectsActiveSeatMissingAgentEndpoint(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(
		repo,
		func(_ tablerunner.ActionProvider, cfg tablerunner.RunnerConfig) Runner { return fakeRunner{cfg: cfg} },
		func(_ string, _ StartRequest, _ ServerConfig) (tablerunner.ActionProvider, error) {
			return fakeProvider{}, nil
		},
		ServerConfig{AdminBearerTokens: map[string]struct{}{"secret": {}}},
	)

	req := httptest.NewRequest(http.MethodPost, "/tables/table-1/start", strings.NewReader(`{
		"hands_to_run": 1,
		"seats": [
			{"seat_no": 1, "stack": 10000, "status": "active"},
			{"seat_no": 2, "stack": 10000, "status": "active", "agent_endpoint": "http://agent.local:9002/callback"}
		]
	}`))
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestStartRejectsMalformedAgentEndpoint(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(
		repo,
		func(_ tablerunner.ActionProvider, cfg tablerunner.RunnerConfig) Runner { return fakeRunner{cfg: cfg} },
		func(_ string, _ StartRequest, _ ServerConfig) (tablerunner.ActionProvider, error) {
			return fakeProvider{}, nil
		},
		ServerConfig{AdminBearerTokens: map[string]struct{}{"secret": {}}},
	)

	req := httptest.NewRequest(http.MethodPost, "/tables/table-1/start", strings.NewReader(`{
		"hands_to_run": 1,
		"seats": [
			{"seat_no": 1, "stack": 10000, "status": "active", "agent_endpoint": "not-a-url"},
			{"seat_no": 2, "stack": 10000, "status": "active", "agent_endpoint": "http://agent.local:9002/callback"}
		]
	}`))
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestStartRejectsInvalidSeatStatus(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(
		repo,
		func(_ tablerunner.ActionProvider, cfg tablerunner.RunnerConfig) Runner { return fakeRunner{cfg: cfg} },
		func(_ string, _ StartRequest, _ ServerConfig) (tablerunner.ActionProvider, error) {
			return fakeProvider{}, nil
		},
		ServerConfig{
			AdminBearerTokens: map[string]struct{}{"secret": {}},
			AllowedAgentHosts: map[string]struct{}{"agent.local:9001": {}, "agent.local:9002": {}},
		},
	)

	req := httptest.NewRequest(http.MethodPost, "/tables/table-1/start", strings.NewReader(`{
		"hands_to_run": 1,
		"seats": [
			{"seat_no": 1, "stack": 10000, "status": "broken", "agent_endpoint": "http://agent.local:9001/callback"},
			{"seat_no": 2, "stack": 10000, "status": "active", "agent_endpoint": "http://agent.local:9002/callback"}
		]
	}`))
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestStartRejectsDisallowedAgentHost(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(
		repo,
		func(_ tablerunner.ActionProvider, cfg tablerunner.RunnerConfig) Runner { return fakeRunner{cfg: cfg} },
		func(_ string, _ StartRequest, _ ServerConfig) (tablerunner.ActionProvider, error) {
			return fakeProvider{}, nil
		},
		ServerConfig{
			AdminBearerTokens: map[string]struct{}{"secret": {}},
			AllowedAgentHosts: map[string]struct{}{"agent.local:9002": {}},
		},
	)

	req := httptest.NewRequest(http.MethodPost, "/tables/table-1/start", strings.NewReader(`{
		"hands_to_run": 1,
		"seats": [
			{"seat_no": 1, "stack": 10000, "status": "active", "agent_endpoint": "http://blocked.local:9001/callback"},
			{"seat_no": 2, "stack": 10000, "status": "active", "agent_endpoint": "http://agent.local:9002/callback"}
		]
	}`))
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestStartAcceptsValidAgentEndpoints(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(
		repo,
		func(_ tablerunner.ActionProvider, cfg tablerunner.RunnerConfig) Runner { return fakeRunner{cfg: cfg} },
		func(_ string, _ StartRequest, _ ServerConfig) (tablerunner.ActionProvider, error) {
			return fakeProvider{}, nil
		},
		ServerConfig{
			AdminBearerTokens: map[string]struct{}{"secret": {}},
			AllowedAgentHosts: map[string]struct{}{"agent.local:9001": {}, "agent.local:9002": {}},
		},
	)

	req := httptest.NewRequest(http.MethodPost, "/tables/table-1/start", strings.NewReader(`{
		"hands_to_run": 1,
		"seats": [
			{"seat_no": 1, "stack": 10000, "status": "active", "agent_endpoint": "http://agent.local:9001/callback"},
			{"seat_no": 2, "stack": 10000, "status": "active", "agent_endpoint": "http://agent.local:9002/callback"}
		]
	}`))
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestStartRejectsOversizedRequestBody(t *testing.T) {
	t.Parallel()

	repo := persistence.NewInMemoryRepository()
	server := NewServer(
		repo,
		func(_ tablerunner.ActionProvider, cfg tablerunner.RunnerConfig) Runner { return fakeRunner{cfg: cfg} },
		func(_ string, _ StartRequest, _ ServerConfig) (tablerunner.ActionProvider, error) {
			return fakeProvider{}, nil
		},
		ServerConfig{AdminBearerTokens: map[string]struct{}{"secret": {}}},
	)

	prefix := `{"hands_to_run":1,"seats":[{"seat_no":1,"stack":10000,"status":"active","agent_endpoint":"http://agent.local/callback","padding":"`
	padding := strings.Repeat("x", maxStartRequestBodyBytes)
	body := prefix + padding + `"},{"seat_no":2,"stack":10000,"status":"active","agent_endpoint":"http://agent.local/callback"}]}`

	req := httptest.NewRequest(http.MethodPost, "/tables/table-1/start", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusRequestEntityTooLarge, w.Code, w.Body.String())
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
		HandID:     "hand-1",
		TableID:    input.TableID,
		HandNo:     input.StartingHand,
		Phase:      domain.HandPhaseBetting,
		Street:     domain.StreetPreflop,
		ActingSeat: seat1,
		Seats: []domain.SeatState{
			domain.NewSeatState(seat1, input.Config.StartingStack),
			domain.NewSeatState(seat2, input.Config.StartingStack),
		},
	}
	if r.cfg.OnHandStart != nil {
		r.cfg.OnHandStart(tablerunner.RunHandInput{
			TableID:    input.TableID,
			HandNo:     input.StartingHand,
			ButtonSeat: input.ButtonSeat,
			Seats:      append([]domain.SeatState(nil), input.Seats...),
			Config:     input.Config,
		}, initial)
	}

	time.Sleep(20 * time.Millisecond)
	final := initial
	final.Phase = domain.HandPhaseComplete
	final.ShowdownAwards = []domain.PotAward{{
		Amount: input.Config.SmallBlind + input.Config.BigBlind,
		Seats:  []domain.SeatNo{seat1},
		Reason: "uncontested",
	}}
	if r.cfg.OnHandComplete != nil {
		r.cfg.OnHandComplete(tablerunner.HandSummary{
			HandNo:        input.StartingHand,
			FinalPhase:    domain.HandPhaseComplete,
			ActionCount:   0,
			FallbackCount: 0,
			FinalState:    final,
		})
	}

	return tablerunner.RunTableResult{
		HandsCompleted: 1,
		FinalButton:    input.ButtonSeat,
		FinalSeats:     append([]domain.SeatState(nil), input.Seats...),
		TotalActions:   0,
		TotalFallbacks: 0,
		HandSummaries: []tablerunner.HandSummary{{
			HandNo:        input.StartingHand,
			FinalPhase:    domain.HandPhaseComplete,
			ActionCount:   0,
			FallbackCount: 0,
			FinalState:    final,
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
