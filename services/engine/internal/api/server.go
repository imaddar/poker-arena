package api

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	"github.com/imaddar/poker-arena/services/engine/internal/persistence"
	"github.com/imaddar/poker-arena/services/engine/internal/tablerunner"
)

// TODO(postgres): Replace the in-memory repository with a Postgres-backed implementation and migrations.
// TODO(auth): Add bearer-token middleware and align endpoint auth with the broader service model.
// TODO(logging): Add structured run/request lifecycle logging and persistence error telemetry.

const (
	maxStartRequestBodyBytes = 1 << 20
	stopWaitTimeout          = 5 * time.Second
)

type Runner interface {
	RunTable(ctx context.Context, input tablerunner.RunTableInput) (tablerunner.RunTableResult, error)
}

type ServerConfig struct {
	AdminBearerTokens     map[string]struct{}
	SeatBearerTokens      map[string]domain.SeatNo
	AllowedAgentHosts     map[string]struct{}
	AllowedCORSOrigins    map[string]struct{}
	DefaultAgentTimeoutMS uint64
	AgentHTTPTimeout      time.Duration
}

type CallerRole string

const (
	CallerRoleAdmin CallerRole = "admin"
	CallerRoleSeat  CallerRole = "seat"
)

type CallerIdentity struct {
	Role  CallerRole
	Seat  *domain.SeatNo
	Token string
}

type tableRun struct {
	cancel context.CancelFunc
	done   chan struct{}
	status persistence.TableRunRecord
}

type Server struct {
	repo            persistence.Repository
	runnerFactory   func(provider tablerunner.ActionProvider, cfg tablerunner.RunnerConfig) Runner
	providerFactory func(tableID string, start StartRequest, cfg ServerConfig) (tablerunner.ActionProvider, error)
	config          ServerConfig

	mu   sync.Mutex
	runs map[string]*tableRun
}

type StartRequest struct {
	HandsToRun   int                 `json:"hands_to_run"`
	StartingHand *uint64             `json:"starting_hand,omitempty"`
	ButtonSeat   *uint8              `json:"button_seat,omitempty"`
	TableConfig  *domain.TableConfig `json:"table_config,omitempty"`
	Seats        []StartSeat         `json:"seats"`
}

type StartSeat struct {
	SeatNo         uint8             `json:"seat_no"`
	Stack          uint32            `json:"stack"`
	Status         domain.SeatStatus `json:"status"`
	AgentEndpoint  string            `json:"agent_endpoint,omitempty"`
	AgentTimeoutMS *uint64           `json:"agent_timeout_ms,omitempty"`
}

type tableStatusResponse struct {
	persistence.TableRunRecord
	HandsPersisted   int `json:"hands_persisted"`
	ActionsPersisted int `json:"actions_persisted"`
}

type tableStateResponse struct {
	Table        persistence.TableRecord     `json:"table"`
	Seats        []persistence.SeatRecord    `json:"seats"`
	LatestRun    *persistence.TableRunRecord `json:"latest_run,omitempty"`
	HandsCount   int                         `json:"hands_count"`
	ActionsCount int                         `json:"actions_count"`
}

type handResponse struct {
	HandID        string            `json:"hand_id"`
	TableID       string            `json:"table_id"`
	HandNo        uint64            `json:"hand_no"`
	StartedAt     time.Time         `json:"started_at"`
	EndedAt       *time.Time        `json:"ended_at,omitempty"`
	FinalPhase    domain.HandPhase  `json:"final_phase"`
	WinnerSummary []domain.PotAward `json:"winner_summary,omitempty"`
}

type actionResponse struct {
	HandID     string            `json:"hand_id"`
	Street     domain.Street     `json:"street"`
	ActingSeat domain.SeatNo     `json:"acting_seat"`
	Action     domain.ActionKind `json:"action"`
	Amount     *uint32           `json:"amount,omitempty"`
	IsFallback bool              `json:"is_fallback"`
	At         time.Time         `json:"at"`
}

type handReplayResponse struct {
	HandID        string            `json:"hand_id"`
	TableID       string            `json:"table_id"`
	HandNo        uint64            `json:"hand_no"`
	StartedAt     time.Time         `json:"started_at"`
	EndedAt       *time.Time        `json:"ended_at,omitempty"`
	FinalPhase    domain.HandPhase  `json:"final_phase"`
	WinnerSummary []domain.PotAward `json:"winner_summary,omitempty"`
	FinalState    domain.HandState  `json:"final_state"`
	Actions       []actionResponse  `json:"actions"`
	Analytics     replayAnalytics   `json:"analytics"`
}

type replayAnalytics struct {
	TotalActions    int                   `json:"total_actions"`
	FallbackActions int                   `json:"fallback_actions"`
	ActionsByStreet map[domain.Street]int `json:"actions_by_street,omitempty"`
	ActionsBySeat   map[domain.SeatNo]int `json:"actions_by_seat,omitempty"`
}

type createUserRequest struct {
	Name  string `json:"name"`
	Token string `json:"token"`
}

type createAgentRequest struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
}

type createAgentVersionRequest struct {
	EndpointURL string          `json:"endpoint_url"`
	ConfigJSON  json.RawMessage `json:"config_json,omitempty"`
}

type createTableRequest struct {
	Name       string  `json:"name"`
	MaxSeats   *uint8  `json:"max_seats,omitempty"`
	SmallBlind *uint32 `json:"small_blind,omitempty"`
	BigBlind   *uint32 `json:"big_blind,omitempty"`
}

type joinTableRequest struct {
	SeatNo         uint8             `json:"seat_no"`
	AgentID        string            `json:"agent_id"`
	AgentVersionID string            `json:"agent_version_id"`
	Stack          uint32            `json:"stack"`
	Status         domain.SeatStatus `json:"status"`
}

func NewServer(
	repo persistence.Repository,
	runnerFactory func(provider tablerunner.ActionProvider, cfg tablerunner.RunnerConfig) Runner,
	providerFactory func(tableID string, start StartRequest, cfg ServerConfig) (tablerunner.ActionProvider, error),
	config ServerConfig,
) *Server {
	return &Server{
		repo:            repo,
		runnerFactory:   runnerFactory,
		providerFactory: providerFactory,
		config:          config,
		runs:            make(map[string]*tableRun),
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if handled := s.handleCORS(w, r); handled {
		return
	}

	identity, ok := s.authenticate(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if r.URL.Path == "/users" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !identity.isAdmin() {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		s.handleCreateUser(w, r)
		return
	}

	if r.URL.Path == "/agents" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !identity.isAdmin() {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		s.handleCreateAgent(w, r)
		return
	}

	if agentID, ok := parseAgentVersionsRoute(r.URL.Path); ok {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !identity.isAdmin() {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		s.handleCreateAgentVersion(w, r, agentID)
		return
	}

	if r.URL.Path == "/tables" {
		if !identity.isAdmin() {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		switch r.Method {
		case http.MethodPost:
			s.handleCreateTable(w, r)
		case http.MethodGet:
			s.handleListTables(w)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if tableID, action, ok := parseTableRoute(r.URL.Path); ok {
		switch {
		case r.Method == http.MethodPost && action == "start":
			if !identity.isAdmin() {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
			s.handleStart(w, r, tableID)
		case r.Method == http.MethodPost && action == "stop":
			if !identity.isAdmin() {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
			s.handleStop(w, tableID)
		case r.Method == http.MethodGet && action == "status":
			if !identity.isAdmin() {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
			s.handleStatus(w, tableID)
		case r.Method == http.MethodGet && action == "state":
			if !identity.isAdmin() {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
			s.handleTableState(w, tableID)
		case r.Method == http.MethodPost && action == "join":
			if !identity.isAdmin() {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
			s.handleJoinTable(w, r, tableID)
		case r.Method == http.MethodGet && action == "hands":
			s.handleHands(w, identity, tableID)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if handID, action, ok := parseHandRoute(r.URL.Path); ok {
		switch {
		case r.Method == http.MethodGet && action == "actions":
			s.handleActions(w, identity, handID)
		case r.Method == http.MethodGet && action == "replay":
			s.handleReplay(w, r, identity, handID)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	writeError(w, http.StatusNotFound, "route not found")
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request, tableID string) {
	if s.repo == nil || s.runnerFactory == nil || s.providerFactory == nil {
		writeError(w, http.StatusInternalServerError, "server is not configured")
		return
	}

	body := http.MaxBytesReader(w, r.Body, maxStartRequestBodyBytes)
	defer body.Close()

	var req StartRequest
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	resolvedReq, statusCode, err := s.hydrateStartRequest(tableID, req)
	if err != nil {
		writeError(w, statusCode, err.Error())
		return
	}

	input, config, buttonSeat, seats, err := validateStartRequest(tableID, resolvedReq, s.config)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.mu.Lock()
	if _, exists := s.runs[tableID]; exists {
		s.mu.Unlock()
		writeError(w, http.StatusConflict, "table is already running")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &tableRun{
		cancel: cancel,
		done:   make(chan struct{}),
		status: persistence.TableRunRecord{
			TableID:        tableID,
			Status:         persistence.TableRunStatusRunning,
			StartedAt:      time.Now().UTC(),
			HandsRequested: resolvedReq.HandsToRun,
			CurrentHandNo:  input.StartingHand,
		},
	}
	s.runs[tableID] = run
	s.mu.Unlock()

	if err := s.repo.UpsertTableRun(run.status); err != nil {
		s.mu.Lock()
		delete(s.runs, tableID)
		s.mu.Unlock()
		writeError(w, http.StatusInternalServerError, "failed to persist run status")
		return
	}

	provider, err := s.providerFactory(tableID, resolvedReq, s.config)
	if err != nil {
		s.failBeforeRun(tableID, run, fmt.Errorf("resolve action provider: %w", err))
		writeError(w, http.StatusInternalServerError, "failed to create action provider")
		return
	}

	handStartedAtByID := make(map[string]time.Time)
	runner := s.runnerFactory(provider, tablerunner.RunnerConfig{
		OnHandStart: func(_ tablerunner.RunHandInput, initial domain.HandState) {
			startedAt := time.Now().UTC()
			if repoErr := s.repo.CreateHand(persistence.HandRecord{
				HandID:     initial.HandID,
				TableID:    initial.TableID,
				HandNo:     initial.HandNo,
				StartedAt:  startedAt,
				FinalPhase: initial.Phase,
				FinalState: initial,
			}); repoErr != nil {
				s.failRun(tableID, run, fmt.Errorf("create hand record: %w", repoErr))
				return
			}
			handStartedAtByID[initial.HandID] = startedAt
			run.status.CurrentHandNo = initial.HandNo
			if repoErr := s.repo.UpsertTableRun(run.status); repoErr != nil {
				s.failRun(tableID, run, fmt.Errorf("update run on hand start: %w", repoErr))
			}
		},
		OnAction: func(_ uint64, state domain.HandState, action domain.Action, isFallback bool) {
			record := persistence.ActionRecord{
				HandID:     state.HandID,
				Street:     state.Street,
				ActingSeat: state.ActingSeat,
				Action:     action.Kind,
				IsFallback: isFallback,
				At:         time.Now().UTC(),
			}
			if action.Amount != nil {
				amount := *action.Amount
				record.Amount = &amount
			}
			if repoErr := s.repo.AppendAction(record); repoErr != nil {
				s.failRun(tableID, run, fmt.Errorf("append action record: %w", repoErr))
			}
		},
		OnHandComplete: func(summary tablerunner.HandSummary) {
			endedAt := time.Now().UTC()
			startedAt, ok := handStartedAtByID[summary.FinalState.HandID]
			if !ok {
				startedAt = run.status.StartedAt
			}
			delete(handStartedAtByID, summary.FinalState.HandID)
			if repoErr := s.repo.CompleteHand(summary.FinalState.HandID, persistence.HandRecord{
				HandID:        summary.FinalState.HandID,
				TableID:       summary.FinalState.TableID,
				HandNo:        summary.HandNo,
				StartedAt:     startedAt,
				EndedAt:       &endedAt,
				FinalPhase:    summary.FinalPhase,
				FinalState:    summary.FinalState,
				WinnerSummary: append([]domain.PotAward(nil), summary.FinalState.ShowdownAwards...),
			}); repoErr != nil {
				s.failRun(tableID, run, fmt.Errorf("complete hand record: %w", repoErr))
				return
			}
			run.status.HandsCompleted++
			run.status.TotalActions += summary.ActionCount
			run.status.TotalFallbacks += summary.FallbackCount
			if repoErr := s.repo.UpsertTableRun(run.status); repoErr != nil {
				s.failRun(tableID, run, fmt.Errorf("update run on hand complete: %w", repoErr))
			}
		},
	})

	go s.runTable(ctx, tableID, run, runner, tablerunner.RunTableInput{
		TableID:      tableID,
		StartingHand: input.StartingHand,
		HandsToRun:   input.HandsToRun,
		ButtonSeat:   buttonSeat,
		Seats:        seats,
		Config:       config,
	})

	writeJSON(w, http.StatusOK, map[string]string{
		"table_id": tableID,
		"status":   string(persistence.TableRunStatusRunning),
	})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if ok := decodeStrictJSON(w, r, &req); !ok {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Token = strings.TrimSpace(req.Token)
	if req.Name == "" || req.Token == "" {
		writeError(w, http.StatusBadRequest, "name and token are required")
		return
	}
	record := persistence.UserRecord{
		ID:        newID("user"),
		Name:      req.Name,
		Token:     req.Token,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.repo.CreateUser(record); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	var req createAgentRequest
	if ok := decodeStrictJSON(w, r, &req); !ok {
		return
	}
	req.UserID = strings.TrimSpace(req.UserID)
	req.Name = strings.TrimSpace(req.Name)
	if req.UserID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "user_id and name are required")
		return
	}
	record := persistence.AgentRecord{
		ID:        newID("agent"),
		UserID:    req.UserID,
		Name:      req.Name,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.repo.CreateAgent(record); err != nil {
		if errors.Is(err, persistence.ErrUserNotFound) {
			writeError(w, http.StatusBadRequest, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create agent")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) handleCreateAgentVersion(w http.ResponseWriter, r *http.Request, agentID string) {
	var req createAgentVersionRequest
	if ok := decodeStrictJSON(w, r, &req); !ok {
		return
	}
	req.EndpointURL = strings.TrimSpace(req.EndpointURL)
	parsedEndpoint, err := url.Parse(req.EndpointURL)
	if err != nil || parsedEndpoint == nil || parsedEndpoint.Host == "" {
		writeError(w, http.StatusBadRequest, "endpoint_url is invalid")
		return
	}
	if parsedEndpoint.Scheme != "http" && parsedEndpoint.Scheme != "https" {
		writeError(w, http.StatusBadRequest, "endpoint_url must use http or https")
		return
	}
	if len(s.config.AllowedAgentHosts) > 0 {
		if _, ok := s.config.AllowedAgentHosts[parsedEndpoint.Host]; !ok {
			writeError(w, http.StatusBadRequest, "endpoint host is not allowlisted")
			return
		}
	}

	configJSON := req.ConfigJSON
	if len(configJSON) == 0 {
		configJSON = json.RawMessage(`{}`)
	}

	var created persistence.AgentVersionRecord
	for version := 1; version <= 10_000; version++ {
		candidate := persistence.AgentVersionRecord{
			ID:          newID("version"),
			AgentID:     agentID,
			Version:     version,
			EndpointURL: req.EndpointURL,
			ConfigJSON:  append([]byte(nil), configJSON...),
			CreatedAt:   time.Now().UTC(),
		}
		err := s.repo.CreateAgentVersion(candidate)
		if err == nil {
			created = candidate
			break
		}
		if errors.Is(err, persistence.ErrAgentNotFound) {
			writeError(w, http.StatusBadRequest, "agent not found")
			return
		}
		if errors.Is(err, persistence.ErrAgentVersionExists) {
			continue
		}
		writeError(w, http.StatusInternalServerError, "failed to create agent version")
		return
	}
	if created.ID == "" {
		writeError(w, http.StatusInternalServerError, "failed to allocate agent version")
		return
	}
	writeJSON(w, http.StatusOK, created)
}

func (s *Server) handleCreateTable(w http.ResponseWriter, r *http.Request) {
	var req createTableRequest
	if ok := decodeStrictJSON(w, r, &req); !ok {
		return
	}

	cfg := domain.DefaultV0TableConfig()
	if req.MaxSeats != nil {
		cfg.MaxSeats = *req.MaxSeats
	}
	if req.SmallBlind != nil {
		cfg.SmallBlind = *req.SmallBlind
	}
	if req.BigBlind != nil {
		cfg.BigBlind = *req.BigBlind
	}
	if err := cfg.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	record := persistence.TableRecord{
		ID:         newID("table"),
		Name:       strings.TrimSpace(req.Name),
		MaxSeats:   cfg.MaxSeats,
		SmallBlind: cfg.SmallBlind,
		BigBlind:   cfg.BigBlind,
		Status:     string(persistence.TableRunStatusIdle),
		CreatedAt:  time.Now().UTC(),
	}
	if record.Name == "" {
		record.Name = record.ID
	}
	if err := s.repo.CreateTable(record); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create table")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) handleListTables(w http.ResponseWriter) {
	tables, err := s.repo.ListTables()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tables")
		return
	}
	writeJSON(w, http.StatusOK, tables)
}

func (s *Server) handleJoinTable(w http.ResponseWriter, r *http.Request, tableID string) {
	var req joinTableRequest
	if ok := decodeStrictJSON(w, r, &req); !ok {
		return
	}
	tableRecord, ok, err := s.repo.GetTable(tableID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load table")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "table not found")
		return
	}
	seatNo, err := domain.NewSeatNo(req.SeatNo, tableRecord.MaxSeats)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Status == "" {
		req.Status = domain.SeatStatusActive
	}
	if !isSeatStatusAllowed(req.Status) {
		writeError(w, http.StatusBadRequest, "invalid seat status")
		return
	}
	if req.Stack == 0 {
		writeError(w, http.StatusBadRequest, "stack must be greater than zero")
		return
	}
	record := persistence.SeatRecord{
		ID:             newID("seat"),
		TableID:        tableID,
		SeatNo:         seatNo,
		AgentID:        strings.TrimSpace(req.AgentID),
		AgentVersionID: strings.TrimSpace(req.AgentVersionID),
		Stack:          req.Stack,
		Status:         req.Status,
		CreatedAt:      time.Now().UTC(),
	}
	if record.AgentID == "" || record.AgentVersionID == "" {
		writeError(w, http.StatusBadRequest, "agent_id and agent_version_id are required")
		return
	}
	if err := s.repo.UpsertSeat(record); err != nil {
		switch {
		case errors.Is(err, persistence.ErrTableNotFound):
			writeError(w, http.StatusNotFound, "table not found")
		case errors.Is(err, persistence.ErrAgentNotFound):
			writeError(w, http.StatusBadRequest, "agent not found")
		case errors.Is(err, persistence.ErrAgentVersionNotFound):
			writeError(w, http.StatusBadRequest, "agent version not found")
		default:
			writeError(w, http.StatusInternalServerError, "failed to join table")
		}
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) handleTableState(w http.ResponseWriter, tableID string) {
	tableRecord, ok, err := s.repo.GetTable(tableID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load table")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "table not found")
		return
	}
	seats, err := s.repo.ListSeats(tableID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load seats")
		return
	}
	run, runOK, err := s.repo.GetTableRun(tableID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load run status")
		return
	}
	hands, err := s.repo.ListHands(tableID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load hands")
		return
	}
	actionCount := 0
	for _, hand := range hands {
		actions, listErr := s.repo.ListActions(hand.HandID)
		if listErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to load actions")
			return
		}
		actionCount += len(actions)
	}
	response := tableStateResponse{
		Table:        tableRecord,
		Seats:        seats,
		HandsCount:   len(hands),
		ActionsCount: actionCount,
	}
	if runOK {
		runCopy := run
		response.LatestRun = &runCopy
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) hydrateStartRequest(tableID string, req StartRequest) (StartRequest, int, error) {
	if len(req.Seats) > 0 {
		return req, http.StatusOK, nil
	}
	tableRecord, ok, err := s.repo.GetTable(tableID)
	if err != nil {
		return StartRequest{}, http.StatusInternalServerError, fmt.Errorf("failed to load table")
	}
	if !ok {
		return StartRequest{}, http.StatusNotFound, fmt.Errorf("table not found")
	}
	seats, err := s.repo.ListSeats(tableID)
	if err != nil {
		return StartRequest{}, http.StatusInternalServerError, fmt.Errorf("failed to load seats")
	}
	if len(seats) == 0 {
		return StartRequest{}, http.StatusBadRequest, fmt.Errorf("table has no seats")
	}
	if req.TableConfig == nil {
		cfg := domain.DefaultV0TableConfig()
		cfg.MaxSeats = tableRecord.MaxSeats
		cfg.SmallBlind = tableRecord.SmallBlind
		cfg.BigBlind = tableRecord.BigBlind
		req.TableConfig = &cfg
	}
	req.Seats = make([]StartSeat, 0, len(seats))
	for _, seat := range seats {
		version, ok, getErr := s.repo.GetAgentVersion(seat.AgentVersionID)
		if getErr != nil {
			return StartRequest{}, http.StatusInternalServerError, fmt.Errorf("failed to load agent version")
		}
		if !ok {
			return StartRequest{}, http.StatusBadRequest, fmt.Errorf("agent version %s not found", seat.AgentVersionID)
		}
		req.Seats = append(req.Seats, StartSeat{
			SeatNo:        uint8(seat.SeatNo),
			Stack:         seat.Stack,
			Status:        seat.Status,
			AgentEndpoint: version.EndpointURL,
		})
	}
	return req, http.StatusOK, nil
}

func (s *Server) handleStop(w http.ResponseWriter, tableID string) {
	s.mu.Lock()
	run, ok := s.runs[tableID]
	s.mu.Unlock()
	if !ok {
		writeError(w, http.StatusConflict, "table is not running")
		return
	}

	run.cancel()
	select {
	case <-run.done:
	case <-time.After(stopWaitTimeout):
		writeError(w, http.StatusGatewayTimeout, "timed out waiting for table to stop")
		return
	}

	status, ok, err := s.repo.GetTableRun(tableID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load table status")
		return
	}
	if !ok {
		writeError(w, http.StatusInternalServerError, "table status missing after stop")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"table_id": tableID,
		"status":   string(status.Status),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, tableID string) {
	record, ok, err := s.repo.GetTableRun(tableID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load table status")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "table status not found")
		return
	}

	hands, err := s.repo.ListHands(tableID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load hands")
		return
	}
	actionCount := 0
	for _, hand := range hands {
		actions, listErr := s.repo.ListActions(hand.HandID)
		if listErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to load actions")
			return
		}
		actionCount += len(actions)
	}

	writeJSON(w, http.StatusOK, tableStatusResponse{
		TableRunRecord:   record,
		HandsPersisted:   len(hands),
		ActionsPersisted: actionCount,
	})
}

func (s *Server) handleHands(w http.ResponseWriter, identity CallerIdentity, tableID string) {
	hands, err := s.repo.ListHands(tableID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load hands")
		return
	}

	filtered := hands
	if identity.Role == CallerRoleSeat {
		filtered = make([]persistence.HandRecord, 0, len(hands))
		for _, hand := range hands {
			if handIncludesSeat(hand, identity.seatNo()) {
				filtered = append(filtered, hand)
			}
		}
	}

	if len(hands) == 0 || (identity.Role == CallerRoleSeat && len(filtered) == 0) {
		_, ok, getErr := s.repo.GetTableRun(tableID)
		if getErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to load table status")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "table status not found")
			return
		}
	}

	response := make([]handResponse, 0, len(filtered))
	for _, hand := range filtered {
		response = append(response, handResponse{
			HandID:        hand.HandID,
			TableID:       hand.TableID,
			HandNo:        hand.HandNo,
			StartedAt:     hand.StartedAt,
			EndedAt:       hand.EndedAt,
			FinalPhase:    hand.FinalPhase,
			WinnerSummary: append([]domain.PotAward(nil), hand.WinnerSummary...),
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleActions(w http.ResponseWriter, identity CallerIdentity, handID string) {
	hand, ok, err := s.repo.GetHand(handID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load hand")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "hand not found")
		return
	}
	if identity.Role == CallerRoleSeat && !handIncludesSeat(hand, identity.seatNo()) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	actions, err := s.repo.ListActions(handID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load actions")
		return
	}
	response := make([]actionResponse, 0, len(actions))
	for _, action := range actions {
		response = append(response, actionResponse{
			HandID:     action.HandID,
			Street:     action.Street,
			ActingSeat: action.ActingSeat,
			Action:     action.Action,
			Amount:     action.Amount,
			IsFallback: action.IsFallback,
			At:         action.At,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleReplay(w http.ResponseWriter, r *http.Request, identity CallerIdentity, handID string) {
	redactHoleCards, err := parseRedactHoleCards(r.URL.Query().Get("redact_hole_cards"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	hand, ok, err := s.repo.GetHand(handID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load hand")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "hand not found")
		return
	}
	if identity.Role == CallerRoleSeat && !handIncludesSeat(hand, identity.seatNo()) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	actions, err := s.repo.ListActions(handID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load actions")
		return
	}
	finalState := cloneHandStateForReplay(hand.FinalState)
	applyReplayVisibility(identity, hand, &finalState)
	if redactHoleCards {
		redactFoldedSeatHoleCards(&finalState)
	}

	analytics := replayAnalytics{
		TotalActions:    len(actions),
		FallbackActions: 0,
		ActionsByStreet: make(map[domain.Street]int),
		ActionsBySeat:   make(map[domain.SeatNo]int),
	}
	actionItems := make([]actionResponse, 0, len(actions))
	for _, action := range actions {
		if action.IsFallback {
			analytics.FallbackActions++
		}
		analytics.ActionsByStreet[action.Street]++
		analytics.ActionsBySeat[action.ActingSeat]++
		actionItems = append(actionItems, actionResponse{
			HandID:     action.HandID,
			Street:     action.Street,
			ActingSeat: action.ActingSeat,
			Action:     action.Action,
			Amount:     action.Amount,
			IsFallback: action.IsFallback,
			At:         action.At,
		})
	}

	writeJSON(w, http.StatusOK, handReplayResponse{
		HandID:        hand.HandID,
		TableID:       hand.TableID,
		HandNo:        hand.HandNo,
		StartedAt:     hand.StartedAt,
		EndedAt:       hand.EndedAt,
		FinalPhase:    hand.FinalPhase,
		WinnerSummary: append([]domain.PotAward(nil), hand.WinnerSummary...),
		FinalState:    finalState,
		Actions:       actionItems,
		Analytics:     analytics,
	})
}

func (s *Server) runTable(ctx context.Context, tableID string, run *tableRun, runner Runner, input tablerunner.RunTableInput) {
	defer func() {
		close(run.done)
		s.mu.Lock()
		delete(s.runs, tableID)
		s.mu.Unlock()
	}()

	result, err := runner.RunTable(ctx, input)
	finalStatus := run.status
	finalStatus.HandsCompleted = result.HandsCompleted
	finalStatus.TotalActions = result.TotalActions
	finalStatus.TotalFallbacks = result.TotalFallbacks
	finalStatus.CurrentHandNo = input.StartingHand + uint64(result.HandsCompleted)
	endedAt := time.Now().UTC()
	finalStatus.EndedAt = &endedAt

	switch {
	case run.status.Status == persistence.TableRunStatusFailed:
		finalStatus.Status = persistence.TableRunStatusFailed
		finalStatus.Error = run.status.Error
	case err == nil:
		finalStatus.Status = persistence.TableRunStatusCompleted
	case errors.Is(err, context.Canceled), errors.Is(err, tablerunner.ErrContextCancelled):
		finalStatus.Status = persistence.TableRunStatusStopped
		finalStatus.Error = err.Error()
	default:
		finalStatus.Status = persistence.TableRunStatusFailed
		finalStatus.Error = err.Error()
	}

	run.status = finalStatus
	_ = s.repo.UpsertTableRun(finalStatus)
}

func (s *Server) failBeforeRun(tableID string, run *tableRun, err error) {
	endedAt := time.Now().UTC()
	run.status.Status = persistence.TableRunStatusFailed
	run.status.EndedAt = &endedAt
	run.status.Error = err.Error()
	_ = s.repo.UpsertTableRun(run.status)
	s.mu.Lock()
	delete(s.runs, tableID)
	s.mu.Unlock()
	close(run.done)
}

func (s *Server) failRun(_ string, run *tableRun, err error) {
	endedAt := time.Now().UTC()
	run.status.Status = persistence.TableRunStatusFailed
	run.status.EndedAt = &endedAt
	run.status.Error = err.Error()
	_ = s.repo.UpsertTableRun(run.status)
	run.cancel()
}

func validateStartRequest(tableID string, req StartRequest, serverCfg ServerConfig) (tablerunner.RunTableInput, domain.TableConfig, domain.SeatNo, []domain.SeatState, error) {
	cfg := domain.DefaultV0TableConfig()
	if req.TableConfig != nil {
		cfg = *req.TableConfig
	}
	if err := cfg.Validate(); err != nil {
		return tablerunner.RunTableInput{}, cfg, 0, nil, err
	}
	if req.HandsToRun <= 0 {
		return tablerunner.RunTableInput{}, cfg, 0, nil, fmt.Errorf("hands_to_run must be greater than zero")
	}
	if len(req.Seats) == 0 {
		return tablerunner.RunTableInput{}, cfg, 0, nil, fmt.Errorf("seats must not be empty")
	}

	seats := make([]domain.SeatState, 0, len(req.Seats))
	seen := make(map[domain.SeatNo]struct{}, len(req.Seats))
	for _, seat := range req.Seats {
		seatNo, err := domain.NewSeatNo(seat.SeatNo, cfg.MaxSeats)
		if err != nil {
			return tablerunner.RunTableInput{}, cfg, 0, nil, err
		}
		if _, exists := seen[seatNo]; exists {
			return tablerunner.RunTableInput{}, cfg, 0, nil, fmt.Errorf("duplicate seat number %d", seatNo)
		}
		seen[seatNo] = struct{}{}
		seatState := domain.NewSeatState(seatNo, seat.Stack)
		if seat.Status != "" {
			if !isSeatStatusAllowed(seat.Status) {
				return tablerunner.RunTableInput{}, cfg, 0, nil, fmt.Errorf("invalid seat status %q for seat %d", seat.Status, seatNo)
			}
			seatState.Status = seat.Status
		}
		if isSeatActiveForStart(seat.Status) {
			parsedEndpoint, err := url.Parse(seat.AgentEndpoint)
			if err != nil || parsedEndpoint == nil || parsedEndpoint.Host == "" {
				return tablerunner.RunTableInput{}, cfg, 0, nil, fmt.Errorf("active seat %d has invalid agent_endpoint", seatNo)
			}
			if parsedEndpoint.Scheme != "http" && parsedEndpoint.Scheme != "https" {
				return tablerunner.RunTableInput{}, cfg, 0, nil, fmt.Errorf("active seat %d has unsupported endpoint scheme %q", seatNo, parsedEndpoint.Scheme)
			}
			if len(serverCfg.AllowedAgentHosts) > 0 {
				if _, ok := serverCfg.AllowedAgentHosts[parsedEndpoint.Host]; !ok {
					return tablerunner.RunTableInput{}, cfg, 0, nil, fmt.Errorf("active seat %d endpoint host %q is not allowlisted", seatNo, parsedEndpoint.Host)
				}
			}
		}
		seats = append(seats, seatState)
	}

	startingHand := uint64(1)
	if req.StartingHand != nil {
		startingHand = *req.StartingHand
	}

	rawButton := uint8(1)
	if req.ButtonSeat != nil {
		rawButton = *req.ButtonSeat
	}
	buttonSeat, err := domain.NewSeatNo(rawButton, cfg.MaxSeats)
	if err != nil {
		return tablerunner.RunTableInput{}, cfg, 0, nil, err
	}

	return tablerunner.RunTableInput{
		TableID:      tableID,
		StartingHand: startingHand,
		HandsToRun:   req.HandsToRun,
		ButtonSeat:   buttonSeat,
		Seats:        seats,
		Config:       cfg,
	}, cfg, buttonSeat, seats, nil
}

func isSeatActiveForStart(status domain.SeatStatus) bool {
	return status == "" || status == domain.SeatStatusActive
}

func isSeatStatusAllowed(status domain.SeatStatus) bool {
	switch status {
	case domain.SeatStatusActive, domain.SeatStatusSittingOut, domain.SeatStatusBusted:
		return true
	default:
		return false
	}
}

func (s *Server) authenticate(r *http.Request) (CallerIdentity, bool) {
	if len(s.config.AdminBearerTokens) == 0 && len(s.config.SeatBearerTokens) == 0 {
		return CallerIdentity{Role: CallerRoleAdmin}, true
	}

	token, ok := parseBearerToken(r.Header.Get("Authorization"))
	if !ok {
		return CallerIdentity{}, false
	}
	if _, exists := s.config.AdminBearerTokens[token]; exists {
		return CallerIdentity{Role: CallerRoleAdmin, Token: token}, true
	}
	if seat, exists := s.config.SeatBearerTokens[token]; exists {
		seatCopy := seat
		return CallerIdentity{Role: CallerRoleSeat, Seat: &seatCopy, Token: token}, true
	}
	return CallerIdentity{}, false
}

func parseBearerToken(authorization string) (string, bool) {
	trimmed := strings.TrimSpace(authorization)
	if !strings.HasPrefix(trimmed, "Bearer ") {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(trimmed, "Bearer "))
	if token == "" {
		return "", false
	}
	return token, true
}

func (s *Server) handleCORS(w http.ResponseWriter, r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}
	if !s.isCORSOriginAllowed(origin) {
		return false
	}

	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Add("Vary", "Origin")

	if r.Method != http.MethodOptions {
		return false
	}
	if strings.TrimSpace(r.Header.Get("Access-Control-Request-Method")) == "" {
		return false
	}

	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type")
	w.WriteHeader(http.StatusNoContent)
	return true
}

func (s *Server) isCORSOriginAllowed(origin string) bool {
	if len(s.config.AllowedCORSOrigins) == 0 {
		return false
	}
	if _, ok := s.config.AllowedCORSOrigins["*"]; ok {
		return true
	}
	_, ok := s.config.AllowedCORSOrigins[origin]
	return ok
}

func parseTableRoute(path string) (tableID string, action string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "tables" {
		return "", "", false
	}
	if parts[1] == "" || parts[2] == "" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

func parseHandRoute(path string) (handID string, action string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "hands" {
		return "", "", false
	}
	if parts[1] == "" || parts[2] == "" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

func parseAgentVersionsRoute(path string) (agentID string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "agents" || parts[2] != "versions" {
		return "", false
	}
	if parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func parseRedactHoleCards(raw string) (bool, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return false, nil
	}
	switch normalized {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid redact_hole_cards query value %q; expected true or false", raw)
	}
}

func cloneHandStateForReplay(state domain.HandState) domain.HandState {
	cloned := state
	cloned.Board = append([]domain.Card(nil), state.Board...)
	cloned.Deck = append([]domain.Card(nil), state.Deck...)
	cloned.Seats = append([]domain.SeatState(nil), state.Seats...)
	cloned.ShowdownAwards = clonePotAwardsForReplay(state.ShowdownAwards)
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

func clonePotAwardsForReplay(awards []domain.PotAward) []domain.PotAward {
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

func redactFoldedSeatHoleCards(state *domain.HandState) {
	folded := make(map[domain.SeatNo]struct{}, len(state.Seats))
	for _, seat := range state.Seats {
		if seat.Folded {
			folded[seat.SeatNo] = struct{}{}
		}
	}
	for i := range state.HoleCards {
		if _, ok := folded[state.HoleCards[i].SeatNo]; ok {
			state.HoleCards[i].Cards = []domain.Card{}
		}
	}
}

func applyReplayVisibility(identity CallerIdentity, hand persistence.HandRecord, state *domain.HandState) {
	if identity.Role != CallerRoleSeat {
		return
	}
	callerSeat := identity.seatNo()
	showdown := isShowdownHand(hand)
	for i := range state.HoleCards {
		seatNo := state.HoleCards[i].SeatNo
		if seatNo == callerSeat || showdown {
			continue
		}
		state.HoleCards[i].Cards = []domain.Card{}
	}
}

func isShowdownHand(hand persistence.HandRecord) bool {
	for _, award := range hand.WinnerSummary {
		if award.Reason == "showdown" {
			return true
		}
	}
	for _, award := range hand.FinalState.ShowdownAwards {
		if award.Reason == "showdown" {
			return true
		}
	}
	return false
}

func handIncludesSeat(hand persistence.HandRecord, seat domain.SeatNo) bool {
	for _, seatState := range hand.FinalState.Seats {
		if seatState.SeatNo == seat {
			return true
		}
	}
	return false
}

func (i CallerIdentity) isAdmin() bool {
	return i.Role == CallerRoleAdmin
}

func (i CallerIdentity) seatNo() domain.SeatNo {
	if i.Seat == nil {
		return 0
	}
	return *i.Seat
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func decodeStrictJSON(w http.ResponseWriter, r *http.Request, dest any) bool {
	body := http.MaxBytesReader(w, r.Body, maxStartRequestBodyBytes)
	defer body.Close()
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(dest); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func newID(prefix string) string {
	random := make([]byte, 8)
	if _, err := cryptorand.Read(random); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("%s-%d-%s", prefix, time.Now().UTC().UnixNano(), hex.EncodeToString(random))
}
