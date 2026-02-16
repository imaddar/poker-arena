package api

import (
	"context"
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
	AuthBearerToken       string
	AllowedAgentHosts     map[string]struct{}
	DefaultAgentTimeoutMS uint64
	AgentHTTPTimeout      time.Duration
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
	if !s.authorized(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if tableID, action, ok := parseTableRoute(r.URL.Path); ok {
		switch {
		case r.Method == http.MethodPost && action == "start":
			s.handleStart(w, r, tableID)
		case r.Method == http.MethodPost && action == "stop":
			s.handleStop(w, tableID)
		case r.Method == http.MethodGet && action == "status":
			s.handleStatus(w, tableID)
		case r.Method == http.MethodGet && action == "hands":
			s.handleHands(w, tableID)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if handID, action, ok := parseHandRoute(r.URL.Path); ok {
		switch {
		case r.Method == http.MethodGet && action == "actions":
			s.handleActions(w, handID)
		case r.Method == http.MethodGet && action == "replay":
			s.handleReplay(w, handID)
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

	input, config, buttonSeat, seats, err := validateStartRequest(tableID, req, s.config)
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
			HandsRequested: req.HandsToRun,
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

	provider, err := s.providerFactory(tableID, req, s.config)
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

func (s *Server) handleHands(w http.ResponseWriter, tableID string) {
	hands, err := s.repo.ListHands(tableID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load hands")
		return
	}
	if len(hands) == 0 {
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

	response := make([]handResponse, 0, len(hands))
	for _, hand := range hands {
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

func (s *Server) handleActions(w http.ResponseWriter, handID string) {
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

func (s *Server) handleReplay(w http.ResponseWriter, handID string) {
	hand, ok, err := s.repo.GetHand(handID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load hand")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "hand not found")
		return
	}

	actions, err := s.repo.ListActions(handID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load actions")
		return
	}
	actionItems := make([]actionResponse, 0, len(actions))
	for _, action := range actions {
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
		FinalState:    hand.FinalState,
		Actions:       actionItems,
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

func (s *Server) authorized(r *http.Request) bool {
	token := strings.TrimSpace(s.config.AuthBearerToken)
	if token == "" {
		return true
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	return auth == "Bearer "+token
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
