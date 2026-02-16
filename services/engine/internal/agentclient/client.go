package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

const (
	ProtocolVersion      = 1
	defaultTimeout       = 2 * time.Second
	defaultActionTimeout = uint64(2000)
	maxResponseBodyBytes = 1 << 20
)

var (
	ErrEndpointNotConfigured = errors.New("agent endpoint not configured")
	ErrRequestTimeout        = errors.New("agent request timeout")
	ErrNetwork               = errors.New("agent network error")
	ErrMalformedResponse     = errors.New("agent response malformed")
	ErrIllegalAgentAction    = errors.New("agent returned illegal action")
	ErrMissingHoleCards      = errors.New("missing acting seat hole cards")
)

type Client struct {
	httpClient *http.Client
}

type Request struct {
	EndpointURL     string
	State           domain.HandState
	ActingSeat      domain.SeatNo
	ActionTimeoutMS uint64
}

type protocolRequest struct {
	ProtocolVersion int               `json:"protocol_version"`
	HandID          string            `json:"hand_id"`
	TableID         string            `json:"table_id"`
	Seat            int               `json:"seat"`
	HoleCards       []string          `json:"hole_cards"`
	Board           []string          `json:"board"`
	Pot             uint32            `json:"pot"`
	ToCall          uint32            `json:"to_call"`
	MinRaiseTo      *uint32           `json:"min_raise_to"`
	Stacks          map[string]uint32 `json:"stacks"`
	Bets            map[string]uint32 `json:"bets"`
	LegalActions    []string          `json:"legal_actions"`
	ActionDeadline  uint64            `json:"action_deadline_ms"`
}

type protocolResponse struct {
	Action string  `json:"action"`
	Amount *uint32 `json:"amount,omitempty"`
}

func New(timeout time.Duration) Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return Client{httpClient: &http.Client{Timeout: timeout}}
}

func (c Client) NextAction(ctx context.Context, req Request) (domain.Action, error) {
	if strings.TrimSpace(req.EndpointURL) == "" {
		return domain.Action{}, ErrEndpointNotConfigured
	}
	if c.httpClient == nil {
		c = New(defaultTimeout)
	}

	payload, legalActionSet, err := buildProtocolRequest(req.State, req.ActingSeat, chooseActionTimeout(req))
	if err != nil {
		return domain.Action{}, err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return domain.Action{}, fmt.Errorf("%w: marshal payload: %v", ErrMalformedResponse, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.EndpointURL, bytes.NewReader(body))
	if err != nil {
		return domain.Action{}, fmt.Errorf("%w: build request: %v", ErrNetwork, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if isTimeoutError(err) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return domain.Action{}, fmt.Errorf("%w: %v", ErrRequestTimeout, err)
		}
		return domain.Action{}, fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return domain.Action{}, fmt.Errorf("%w: status %d", ErrNetwork, resp.StatusCode)
	}

	limitedBody := io.LimitReader(resp.Body, maxResponseBodyBytes+1)
	decoder := json.NewDecoder(limitedBody)

	var dto protocolResponse
	if err := decoder.Decode(&dto); err != nil {
		return domain.Action{}, fmt.Errorf("%w: decode: %v", ErrMalformedResponse, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return domain.Action{}, fmt.Errorf("%w: response body has trailing data", ErrMalformedResponse)
	}

	action, err := parseAndValidateProtocolResponse(dto, legalActionSet)
	if err != nil {
		return domain.Action{}, err
	}
	return action, nil
}

func chooseActionTimeout(req Request) uint64 {
	if req.ActionTimeoutMS > 0 {
		return req.ActionTimeoutMS
	}
	return defaultActionTimeout
}

func buildProtocolRequest(state domain.HandState, actingSeat domain.SeatNo, timeoutMS uint64) (protocolRequest, map[domain.ActionKind]struct{}, error) {
	acting, ok := seatByNo(state.Seats, actingSeat)
	if !ok {
		return protocolRequest{}, nil, fmt.Errorf("%w: acting seat %d not found", ErrMalformedResponse, actingSeat)
	}

	holeCards, ok := holeCardsForSeat(state.HoleCards, actingSeat)
	if !ok {
		return protocolRequest{}, nil, fmt.Errorf("%w: seat %d", ErrMissingHoleCards, actingSeat)
	}
	if len(holeCards) != 2 {
		return protocolRequest{}, nil, fmt.Errorf("%w: seat %d has %d cards", ErrMissingHoleCards, actingSeat, len(holeCards))
	}

	toCall := uint32(0)
	if state.CurrentBet > acting.CommittedInRound {
		toCall = state.CurrentBet - acting.CommittedInRound
	}

	legalKinds := deriveLegalActions(state, acting, toCall)
	legalActionSet := make(map[domain.ActionKind]struct{}, len(legalKinds))
	legalActions := make([]string, 0, len(legalKinds))
	for _, kind := range legalKinds {
		legalActionSet[kind] = struct{}{}
		legalActions = append(legalActions, string(kind))
	}

	var minRaiseTo *uint32
	if _, ok := legalActionSet[domain.ActionRaise]; ok {
		value := state.MinRaiseTo
		minRaiseTo = &value
	}

	payload := protocolRequest{
		ProtocolVersion: ProtocolVersion,
		HandID:          state.HandID,
		TableID:         state.TableID,
		Seat:            int(actingSeat),
		HoleCards: []string{
			formatCardASCII(holeCards[0]),
			formatCardASCII(holeCards[1]),
		},
		Board:          make([]string, 0, len(state.Board)),
		Pot:            state.Pot,
		ToCall:         toCall,
		MinRaiseTo:     minRaiseTo,
		Stacks:         make(map[string]uint32, len(state.Seats)),
		Bets:           make(map[string]uint32, len(state.Seats)),
		LegalActions:   legalActions,
		ActionDeadline: timeoutMS,
	}

	for _, card := range state.Board {
		payload.Board = append(payload.Board, formatCardASCII(card))
	}
	for _, seat := range state.Seats {
		key := strconv.Itoa(int(seat.SeatNo))
		payload.Stacks[key] = seat.Stack
		payload.Bets[key] = seat.CommittedInRound
	}

	return payload, legalActionSet, nil
}

func parseAndValidateProtocolResponse(dto protocolResponse, legal map[domain.ActionKind]struct{}) (domain.Action, error) {
	kind := domain.ActionKind(dto.Action)
	if _, ok := legal[kind]; !ok {
		return domain.Action{}, fmt.Errorf("%w: action %q not legal", ErrIllegalAgentAction, dto.Action)
	}

	switch kind {
	case domain.ActionBet, domain.ActionRaise:
		if dto.Amount == nil || *dto.Amount == 0 {
			return domain.Action{}, fmt.Errorf("%w: %s requires positive amount", ErrIllegalAgentAction, kind)
		}
		action, err := domain.NewAction(kind, dto.Amount)
		if err != nil {
			return domain.Action{}, fmt.Errorf("%w: %v", ErrIllegalAgentAction, err)
		}
		return action, nil
	default:
		if dto.Amount != nil {
			return domain.Action{}, fmt.Errorf("%w: amount not allowed for %s", ErrIllegalAgentAction, kind)
		}
		action, err := domain.NewAction(kind, nil)
		if err != nil {
			return domain.Action{}, fmt.Errorf("%w: %v", ErrIllegalAgentAction, err)
		}
		return action, nil
	}
}

func deriveLegalActions(state domain.HandState, acting domain.SeatState, toCall uint32) []domain.ActionKind {
	actions := []domain.ActionKind{domain.ActionFold}
	if toCall == 0 {
		actions = append(actions, domain.ActionCheck)
		if acting.Stack > 0 && state.CurrentBet == 0 {
			actions = append(actions, domain.ActionBet)
		}
		return actions
	}

	actions = append(actions, domain.ActionCall)
	if state.CurrentBet > 0 && acting.Stack > toCall {
		actions = append(actions, domain.ActionRaise)
	}
	return actions
}

func formatCardASCII(card domain.Card) string {
	return formatRankASCII(card.Rank) + formatSuitASCII(card.Suit)
}

func formatRankASCII(rank domain.Rank) string {
	switch uint8(rank) {
	case 14:
		return "A"
	case 13:
		return "K"
	case 12:
		return "Q"
	case 11:
		return "J"
	case 10:
		return "T"
	default:
		return strconv.FormatUint(uint64(rank), 10)
	}
}

func formatSuitASCII(suit domain.Suit) string {
	switch suit {
	case domain.SuitClubs:
		return "c"
	case domain.SuitDiamonds:
		return "d"
	case domain.SuitHearts:
		return "h"
	case domain.SuitSpades:
		return "s"
	default:
		return "?"
	}
}

func seatByNo(seats []domain.SeatState, seatNo domain.SeatNo) (domain.SeatState, bool) {
	for _, seat := range seats {
		if seat.SeatNo == seatNo {
			return seat, true
		}
	}
	return domain.SeatState{}, false
}

func holeCardsForSeat(cards []domain.SeatCards, seatNo domain.SeatNo) ([]domain.Card, bool) {
	for _, seatCards := range cards {
		if seatCards.SeatNo == seatNo {
			return append([]domain.Card(nil), seatCards.Cards...), true
		}
	}
	return nil, false
}

func isTimeoutError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return false
}
