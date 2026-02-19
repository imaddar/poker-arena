package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/imaddar/poker-arena/services/engine/internal/agentclient"
	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	"github.com/imaddar/poker-arena/services/engine/internal/persistence"
	"github.com/imaddar/poker-arena/services/engine/internal/tablerunner"
)

func TestAgentRun_CompletesAndPersistsHistory(t *testing.T) {
	t.Parallel()

	agentA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAgentAction(t, w, r, false)
	}))
	defer agentA.Close()
	agentB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAgentAction(t, w, r, false)
	}))
	defer agentB.Close()

	repo := persistence.NewInMemoryRepository()
	token := "integration-token"
	server := newIntegrationServer(t, repo, token, []string{mustHost(t, agentA.URL), mustHost(t, agentB.URL)}, 250*time.Millisecond)

	start := StartRequest{
		HandsToRun: 3,
		Seats: []StartSeat{
			{SeatNo: 1, Stack: 10_000, Status: domain.SeatStatusActive, AgentEndpoint: agentA.URL + "/callback"},
			{SeatNo: 2, Stack: 10_000, Status: domain.SeatStatusActive, AgentEndpoint: agentB.URL + "/callback"},
		},
	}
	startTable(t, server, token, "table-agent-happy", start, http.StatusOK)

	status := waitForTerminalStatus(t, server, token, "table-agent-happy", 3*time.Second)
	if status.Status != persistence.TableRunStatusCompleted {
		t.Fatalf("expected completed status, got %q err=%q", status.Status, status.Error)
	}
	if status.HandsCompleted != 3 {
		t.Fatalf("expected 3 hands completed, got %d", status.HandsCompleted)
	}
	if status.TotalActions == 0 {
		t.Fatal("expected actions to be persisted")
	}
	if status.TotalFallbacks != 0 {
		t.Fatalf("expected zero fallbacks, got %d", status.TotalFallbacks)
	}

	hands := listHands(t, server, token, "table-agent-happy")
	if len(hands) != 3 {
		t.Fatalf("expected 3 hand records, got %d", len(hands))
	}
	for _, hand := range hands {
		actions := listActions(t, server, token, hand.HandID)
		if len(actions) == 0 {
			t.Fatalf("expected actions for hand %s", hand.HandID)
		}
		for _, action := range actions {
			if action.IsFallback {
				t.Fatalf("expected no fallback actions in happy path, found one in hand %s", hand.HandID)
			}
		}
	}

	replay := getReplay(t, server, token, hands[0].HandID)
	if len(replay.Actions) == 0 {
		t.Fatalf("expected replay actions for hand %s", hands[0].HandID)
	}
	if replay.FinalState.HandID != hands[0].HandID {
		t.Fatalf("expected replay final_state.hand_id %q, got %q", hands[0].HandID, replay.FinalState.HandID)
	}
	if replay.Analytics.TotalActions != len(replay.Actions) {
		t.Fatalf("expected analytics total_actions=%d, got %d", len(replay.Actions), replay.Analytics.TotalActions)
	}
	if replay.Analytics.FallbackActions != 0 {
		t.Fatalf("expected no fallback actions in happy-path replay analytics, got %d", replay.Analytics.FallbackActions)
	}
}

func TestAgentRun_CanStartFromPersistedTableResources(t *testing.T) {
	t.Parallel()

	agentA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAgentAction(t, w, r, false)
	}))
	defer agentA.Close()
	agentB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAgentAction(t, w, r, false)
	}))
	defer agentB.Close()

	repo := persistence.NewInMemoryRepository()
	adminToken := "admin-token"
	server := newIntegrationServer(t, repo, adminToken, []string{mustHost(t, agentA.URL), mustHost(t, agentB.URL)}, 250*time.Millisecond)

	user := postJSON(t, server, adminToken, http.MethodPost, "/users", map[string]any{
		"name":  "u1",
		"token": "user-token",
	}, http.StatusOK)
	userID := user["ID"].(string)
	agent1 := postJSON(t, server, adminToken, http.MethodPost, "/agents", map[string]any{
		"user_id": userID,
		"name":    "agent-1",
	}, http.StatusOK)
	agent1ID := agent1["ID"].(string)
	agent2 := postJSON(t, server, adminToken, http.MethodPost, "/agents", map[string]any{
		"user_id": userID,
		"name":    "agent-2",
	}, http.StatusOK)
	agent2ID := agent2["ID"].(string)

	version1 := postJSON(t, server, adminToken, http.MethodPost, "/agents/"+agent1ID+"/versions", map[string]any{
		"endpoint_url": agentA.URL + "/callback",
	}, http.StatusOK)
	version2 := postJSON(t, server, adminToken, http.MethodPost, "/agents/"+agent2ID+"/versions", map[string]any{
		"endpoint_url": agentB.URL + "/callback",
	}, http.StatusOK)
	table := postJSON(t, server, adminToken, http.MethodPost, "/tables", map[string]any{
		"name":        "table-resource",
		"max_seats":   6,
		"small_blind": 50,
		"big_blind":   100,
	}, http.StatusOK)
	tableID := table["id"].(string)

	_ = postJSON(t, server, adminToken, http.MethodPost, "/tables/"+tableID+"/join", map[string]any{
		"seat_no":          1,
		"agent_id":         agent1ID,
		"agent_version_id": version1["ID"].(string),
		"stack":            10000,
		"status":           "active",
	}, http.StatusOK)
	_ = postJSON(t, server, adminToken, http.MethodPost, "/tables/"+tableID+"/join", map[string]any{
		"seat_no":          2,
		"agent_id":         agent2ID,
		"agent_version_id": version2["ID"].(string),
		"stack":            10000,
		"status":           "active",
	}, http.StatusOK)

	startTable(t, server, adminToken, tableID, StartRequest{HandsToRun: 2}, http.StatusOK)
	status := waitForTerminalStatus(t, server, adminToken, tableID, 3*time.Second)
	if status.Status != persistence.TableRunStatusCompleted {
		t.Fatalf("expected completed status, got %q err=%q", status.Status, status.Error)
	}
	if status.HandsCompleted != 2 {
		t.Fatalf("expected 2 hands completed, got %d", status.HandsCompleted)
	}
}

func TestAgentRun_TimeoutTriggersFallbackAndPersistsIt(t *testing.T) {
	t.Parallel()

	agentFast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAgentAction(t, w, r, false)
	}))
	defer agentFast.Close()
	agentSlow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(250 * time.Millisecond)
		writeAgentAction(t, w, r, false)
	}))
	defer agentSlow.Close()

	repo := persistence.NewInMemoryRepository()
	token := "integration-token"
	server := newIntegrationServer(t, repo, token, []string{mustHost(t, agentFast.URL), mustHost(t, agentSlow.URL)}, 50*time.Millisecond)

	start := StartRequest{
		HandsToRun: 1,
		Seats: []StartSeat{
			{SeatNo: 1, Stack: 10_000, Status: domain.SeatStatusActive, AgentEndpoint: agentFast.URL + "/callback"},
			{SeatNo: 2, Stack: 10_000, Status: domain.SeatStatusActive, AgentEndpoint: agentSlow.URL + "/callback"},
		},
	}
	startTable(t, server, token, "table-agent-timeout", start, http.StatusOK)

	status := waitForTerminalStatus(t, server, token, "table-agent-timeout", 3*time.Second)
	if status.Status != persistence.TableRunStatusCompleted {
		t.Fatalf("expected completed status, got %q err=%q", status.Status, status.Error)
	}
	if status.TotalFallbacks == 0 {
		t.Fatal("expected fallback actions due to timeout")
	}

	hands := listHands(t, server, token, "table-agent-timeout")
	if len(hands) == 0 {
		t.Fatal("expected at least one hand")
	}
	foundFallback := false
	for _, hand := range hands {
		for _, action := range listActions(t, server, token, hand.HandID) {
			if action.IsFallback {
				foundFallback = true
				break
			}
		}
	}
	if !foundFallback {
		t.Fatal("expected persisted fallback action record")
	}
	replay := getReplay(t, server, token, hands[0].HandID)
	if replay.Analytics.FallbackActions == 0 {
		t.Fatal("expected replay analytics to include fallback actions")
	}
}

func TestAgentRun_IllegalActionTriggersFallback(t *testing.T) {
	t.Parallel()

	agentLegal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAgentAction(t, w, r, false)
	}))
	defer agentLegal.Close()
	agentIllegal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"action": "raise",
			"amount": uint32(0),
		})
	}))
	defer agentIllegal.Close()

	repo := persistence.NewInMemoryRepository()
	token := "integration-token"
	server := newIntegrationServer(t, repo, token, []string{mustHost(t, agentLegal.URL), mustHost(t, agentIllegal.URL)}, 250*time.Millisecond)

	start := StartRequest{
		HandsToRun: 1,
		Seats: []StartSeat{
			{SeatNo: 1, Stack: 10_000, Status: domain.SeatStatusActive, AgentEndpoint: agentLegal.URL + "/callback"},
			{SeatNo: 2, Stack: 10_000, Status: domain.SeatStatusActive, AgentEndpoint: agentIllegal.URL + "/callback"},
		},
	}
	startTable(t, server, token, "table-agent-illegal", start, http.StatusOK)

	status := waitForTerminalStatus(t, server, token, "table-agent-illegal", 3*time.Second)
	if status.Status != persistence.TableRunStatusCompleted {
		t.Fatalf("expected completed status, got %q err=%q", status.Status, status.Error)
	}
	if status.TotalFallbacks == 0 {
		t.Fatal("expected fallback actions due to illegal agent response")
	}
}

func newIntegrationServer(
	t *testing.T,
	repo persistence.Repository,
	adminToken string,
	allowlist []string,
	clientTimeout time.Duration,
) *Server {
	return newIntegrationServerWithTokens(t, repo, adminToken, nil, allowlist, clientTimeout)
}

func newIntegrationServerWithTokens(
	t *testing.T,
	repo persistence.Repository,
	adminToken string,
	seatTokens map[string]domain.SeatNo,
	allowlist []string,
	clientTimeout time.Duration,
) *Server {
	t.Helper()

	allowedHosts := make(map[string]struct{}, len(allowlist))
	for _, host := range allowlist {
		allowedHosts[host] = struct{}{}
	}

	config := ServerConfig{
		AdminBearerTokens:     map[string]struct{}{adminToken: {}},
		SeatBearerTokens:      seatTokens,
		AllowedAgentHosts:     allowedHosts,
		DefaultAgentTimeoutMS: domain.DefaultActionTimeoutMS,
		AgentHTTPTimeout:      clientTimeout,
	}

	return NewServer(
		repo,
		func(provider tablerunner.ActionProvider, cfg tablerunner.RunnerConfig) Runner {
			return tablerunner.New(provider, cfg)
		},
		func(_ string, start StartRequest, cfg ServerConfig) (tablerunner.ActionProvider, error) {
			maxSeats := domain.DefaultV0TableConfig().MaxSeats
			if start.TableConfig != nil {
				maxSeats = start.TableConfig.MaxSeats
			}
			endpoints := make(map[domain.SeatNo]string, len(start.Seats))
			for _, seat := range start.Seats {
				seatNo, err := domain.NewSeatNo(seat.SeatNo, maxSeats)
				if err != nil {
					return nil, err
				}
				if seat.Status == "" || seat.Status == domain.SeatStatusActive {
					endpoints[seatNo] = strings.TrimSpace(seat.AgentEndpoint)
				}
			}
			return agentclient.ActionProvider{
				Client:           agentclient.New(cfg.AgentHTTPTimeout),
				Endpoints:        staticSeatEndpoints{endpoints: endpoints},
				DefaultTimeoutMS: cfg.DefaultAgentTimeoutMS,
			}, nil
		},
		config,
	)
}

func TestSeatToken_HistoryAllowed_ControlForbidden(t *testing.T) {
	t.Parallel()

	agentA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAgentAction(t, w, r, false)
	}))
	defer agentA.Close()
	agentB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAgentAction(t, w, r, false)
	}))
	defer agentB.Close()

	repo := persistence.NewInMemoryRepository()
	adminToken := "admin-token"
	seatToken := "seat-1-token"
	server := newIntegrationServerWithTokens(
		t,
		repo,
		adminToken,
		map[string]domain.SeatNo{seatToken: 1},
		[]string{mustHost(t, agentA.URL), mustHost(t, agentB.URL)},
		250*time.Millisecond,
	)

	if err := repo.CreateTable(persistence.TableRecord{
		ID:         "table-seat-latest-empty",
		Name:       "table-seat-latest-empty",
		MaxSeats:   6,
		SmallBlind: 50,
		BigBlind:   100,
		Status:     string(persistence.TableRunStatusIdle),
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateTable failed: %v", err)
	}

	start := StartRequest{
		HandsToRun: 1,
		Seats: []StartSeat{
			{SeatNo: 1, Stack: 10_000, Status: domain.SeatStatusActive, AgentEndpoint: agentA.URL + "/callback"},
			{SeatNo: 2, Stack: 10_000, Status: domain.SeatStatusActive, AgentEndpoint: agentB.URL + "/callback"},
		},
	}
	startTable(t, server, adminToken, "table-seat-history", start, http.StatusOK)
	_ = waitForTerminalStatus(t, server, adminToken, "table-seat-history", 3*time.Second)

	hands := listHands(t, server, seatToken, "table-seat-history")
	if len(hands) == 0 {
		t.Fatal("expected seat token to see participating hands")
	}
	replay := getReplay(t, server, seatToken, hands[0].HandID)
	if replay.HandID != hands[0].HandID {
		t.Fatalf("expected replay hand %q, got %q", hands[0].HandID, replay.HandID)
	}

	req := httptest.NewRequest(http.MethodGet, "/tables/table-seat-history/status", nil)
	req.Header.Set("Authorization", "Bearer "+seatToken)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected seat token status request to be forbidden, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestSeatToken_ReplayRedactsOpponentHoleCardsForNonShowdown(t *testing.T) {
	t.Parallel()

	agentA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAgentAction(t, w, r, false)
	}))
	defer agentA.Close()
	agentB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAgentAction(t, w, r, true)
	}))
	defer agentB.Close()

	repo := persistence.NewInMemoryRepository()
	adminToken := "admin-token"
	seatToken := "seat-1-token"
	server := newIntegrationServerWithTokens(
		t,
		repo,
		adminToken,
		map[string]domain.SeatNo{seatToken: 1},
		[]string{mustHost(t, agentA.URL), mustHost(t, agentB.URL)},
		250*time.Millisecond,
	)

	start := StartRequest{
		HandsToRun: 1,
		Seats: []StartSeat{
			{SeatNo: 1, Stack: 10_000, Status: domain.SeatStatusActive, AgentEndpoint: agentA.URL + "/callback"},
			{SeatNo: 2, Stack: 10_000, Status: domain.SeatStatusActive, AgentEndpoint: agentB.URL + "/callback"},
		},
	}
	startTable(t, server, adminToken, "table-seat-redact", start, http.StatusOK)
	status := waitForTerminalStatus(t, server, adminToken, "table-seat-redact", 3*time.Second)
	if status.Status != persistence.TableRunStatusCompleted {
		t.Fatalf("expected completed status, got %q", status.Status)
	}

	hands := listHands(t, server, seatToken, "table-seat-redact")
	if len(hands) == 0 {
		t.Fatal("expected hand history for seat token")
	}
	replay := getReplay(t, server, seatToken, hands[0].HandID)
	var ownCards int
	var oppCards int
	for _, seatCards := range replay.FinalState.HoleCards {
		if seatCards.SeatNo == 1 {
			ownCards = len(seatCards.Cards)
		}
		if seatCards.SeatNo == 2 {
			oppCards = len(seatCards.Cards)
		}
	}
	if ownCards != 2 {
		t.Fatalf("expected own hole cards to remain visible, got %d", ownCards)
	}
	if oppCards != 0 {
		t.Fatalf("expected opponent hole cards to be redacted in non-showdown hand, got %d", oppCards)
	}
}

func TestSeatToken_LatestReplayReturnsTableOnlyWhenSeatNotInAnyHand(t *testing.T) {
	t.Parallel()

	agentA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAgentAction(t, w, r, false)
	}))
	defer agentA.Close()
	agentB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAgentAction(t, w, r, false)
	}))
	defer agentB.Close()

	repo := persistence.NewInMemoryRepository()
	adminToken := "admin-token"
	seatToken := "seat-3-token"
	server := newIntegrationServerWithTokens(
		t,
		repo,
		adminToken,
		map[string]domain.SeatNo{seatToken: 3},
		[]string{mustHost(t, agentA.URL), mustHost(t, agentB.URL)},
		250*time.Millisecond,
	)
	if err := repo.CreateTable(persistence.TableRecord{
		ID:         "table-seat-latest-empty",
		Name:       "table-seat-latest-empty",
		MaxSeats:   6,
		SmallBlind: 50,
		BigBlind:   100,
		Status:     string(persistence.TableRunStatusIdle),
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateTable failed: %v", err)
	}

	start := StartRequest{
		HandsToRun: 1,
		Seats: []StartSeat{
			{SeatNo: 1, Stack: 10_000, Status: domain.SeatStatusActive, AgentEndpoint: agentA.URL + "/callback"},
			{SeatNo: 2, Stack: 10_000, Status: domain.SeatStatusActive, AgentEndpoint: agentB.URL + "/callback"},
		},
	}
	startTable(t, server, adminToken, "table-seat-latest-empty", start, http.StatusOK)
	_ = waitForTerminalStatus(t, server, adminToken, "table-seat-latest-empty", 3*time.Second)

	payload := getLatestReplayForTable(t, server, seatToken, "table-seat-latest-empty")
	if payload.Table.ID != "table-seat-latest-empty" {
		t.Fatalf("expected table id table-seat-latest-empty, got %q", payload.Table.ID)
	}
	if payload.LatestHand != nil {
		t.Fatalf("expected latest_hand to be omitted, got %+v", payload.LatestHand)
	}
	if payload.Replay != nil {
		t.Fatalf("expected replay to be omitted, got %+v", payload.Replay)
	}
}

type staticSeatEndpoints struct {
	endpoints map[domain.SeatNo]string
}

func (p staticSeatEndpoints) EndpointForSeat(_ domain.HandState, seat domain.SeatNo) (string, error) {
	endpoint, ok := p.endpoints[seat]
	if !ok || endpoint == "" {
		return "", fmt.Errorf("%w: seat %d", agentclient.ErrEndpointNotConfigured, seat)
	}
	return endpoint, nil
}

func startTable(t *testing.T, server *Server, token string, tableID string, start StartRequest, wantStatus int) {
	t.Helper()

	body, err := json.Marshal(start)
	if err != nil {
		t.Fatalf("marshal start request failed: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/tables/"+tableID+"/start", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != wantStatus {
		t.Fatalf("expected status %d, got %d body=%s", wantStatus, w.Code, w.Body.String())
	}
}

func waitForTerminalStatus(t *testing.T, server *Server, token string, tableID string, timeout time.Duration) tableStatusResponse {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status := getStatus(t, server, token, tableID)
		if status.Status == persistence.TableRunStatusCompleted ||
			status.Status == persistence.TableRunStatusStopped ||
			status.Status == persistence.TableRunStatusFailed {
			return status
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for terminal status for table %s", tableID)
	return tableStatusResponse{}
}

func getStatus(t *testing.T, server *Server, token string, tableID string) tableStatusResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/tables/"+tableID+"/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status request failed: %d body=%s", w.Code, w.Body.String())
	}

	var status tableStatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status failed: %v", err)
	}
	return status
}

func listHands(t *testing.T, server *Server, token string, tableID string) []handResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/tables/"+tableID+"/hands", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("hands request failed: %d body=%s", w.Code, w.Body.String())
	}

	var hands []handResponse
	if err := json.Unmarshal(w.Body.Bytes(), &hands); err != nil {
		t.Fatalf("decode hands failed: %v", err)
	}
	return hands
}

func listActions(t *testing.T, server *Server, token string, handID string) []actionResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/hands/"+handID+"/actions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("actions request failed: %d body=%s", w.Code, w.Body.String())
	}

	var actions []actionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &actions); err != nil {
		t.Fatalf("decode actions failed: %v", err)
	}
	return actions
}

func getReplay(t *testing.T, server *Server, token string, handID string) handReplayResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/hands/"+handID+"/replay", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("replay request failed: %d body=%s", w.Code, w.Body.String())
	}

	var replay handReplayResponse
	if err := json.Unmarshal(w.Body.Bytes(), &replay); err != nil {
		t.Fatalf("decode replay failed: %v", err)
	}
	return replay
}

func getLatestReplayForTable(t *testing.T, server *Server, token string, tableID string) latestTableReplayResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/tables/"+tableID+"/replay/latest", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("latest replay request failed: %d body=%s", w.Code, w.Body.String())
	}

	var payload latestTableReplayResponse
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode latest replay failed: %v", err)
	}
	return payload
}

func mustHost(t *testing.T, rawURL string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url parse failed: %v", err)
	}
	return parsed.Host
}

func postJSON(
	t *testing.T,
	server *Server,
	token string,
	method string,
	path string,
	payload map[string]any,
	wantStatus int,
) map[string]any {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != wantStatus {
		t.Fatalf("request %s %s failed: want=%d got=%d body=%s", method, path, wantStatus, w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	return out
}

func writeAgentAction(t *testing.T, w http.ResponseWriter, r *http.Request, forceFold bool) {
	t.Helper()

	var payload struct {
		ToCall       uint32   `json:"to_call"`
		LegalActions []string `json:"legal_actions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		t.Fatalf("decode agent request failed: %v", err)
	}

	action := "fold"
	if forceFold {
		action = chooseLegal(payload.LegalActions, "fold", "check", "call")
	} else if payload.ToCall > 0 {
		action = chooseLegal(payload.LegalActions, "call", "fold")
	} else {
		action = chooseLegal(payload.LegalActions, "check", "fold")
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"action": action})
}

func chooseLegal(legal []string, order ...string) string {
	set := make(map[string]struct{}, len(legal))
	for _, action := range legal {
		set[action] = struct{}{}
	}
	for _, candidate := range order {
		if _, ok := set[candidate]; ok {
			return candidate
		}
	}
	return "fold"
}

func TestStopDuringActiveRun_PreservesPartialHistory(t *testing.T) {
	t.Parallel()

	agentA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(80 * time.Millisecond)
		writeAgentAction(t, w, r, false)
	}))
	defer agentA.Close()
	agentB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(80 * time.Millisecond)
		writeAgentAction(t, w, r, false)
	}))
	defer agentB.Close()

	repo := persistence.NewInMemoryRepository()
	token := "integration-token"
	server := newIntegrationServer(t, repo, token, []string{mustHost(t, agentA.URL), mustHost(t, agentB.URL)}, 250*time.Millisecond)

	start := StartRequest{
		HandsToRun: 8,
		Seats: []StartSeat{
			{SeatNo: 1, Stack: 10_000, Status: domain.SeatStatusActive, AgentEndpoint: agentA.URL + "/callback"},
			{SeatNo: 2, Stack: 10_000, Status: domain.SeatStatusActive, AgentEndpoint: agentB.URL + "/callback"},
		},
	}
	startTable(t, server, token, "table-agent-stop", start, http.StatusOK)

	time.Sleep(120 * time.Millisecond)
	req := httptest.NewRequest(http.MethodPost, "/tables/table-agent-stop/stop", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("stop failed: %d body=%s", w.Code, w.Body.String())
	}

	status := getStatus(t, server, token, "table-agent-stop")
	if status.Status != persistence.TableRunStatusStopped {
		t.Fatalf("expected stopped status, got %q", status.Status)
	}

	hands := listHands(t, server, token, "table-agent-stop")
	if len(hands) == 0 {
		t.Fatal("expected partial history with at least one hand")
	}
}
