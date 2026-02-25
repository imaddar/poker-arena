package api

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

const defaultHumanActionTimeoutMS = uint64(2000)

type HumanPendingAction struct {
	TableID          string   `json:"table_id"`
	HandID           string   `json:"hand_id"`
	SeatNo           int      `json:"seat_no"`
	HoleCards        []string `json:"hole_cards"`
	Board            []string `json:"board"`
	Pot              uint32   `json:"pot"`
	ToCall           uint32   `json:"to_call"`
	MinRaiseTo       *uint32  `json:"min_raise_to,omitempty"`
	LegalActions     []string `json:"legal_actions"`
	ActionDeadlineMS uint64   `json:"action_deadline_ms"`
}

type HumanActionRequest struct {
	SeatNo int     `json:"seat_no"`
	Action string  `json:"action"`
	Amount *uint32 `json:"amount,omitempty"`
}

type pendingKey struct {
	tableID string
	seatNo  domain.SeatNo
}

type pendingAction struct {
	payload HumanPendingAction
	legal   map[domain.ActionKind]struct{}
	out     chan domain.Action
}

type HumanActionHub struct {
	mu      sync.Mutex
	pending map[pendingKey]pendingAction
}

func NewHumanActionHub() *HumanActionHub {
	return &HumanActionHub{
		pending: make(map[pendingKey]pendingAction),
	}
}

var defaultHumanActionHub = NewHumanActionHub()

func DefaultHumanActionHub() *HumanActionHub {
	return defaultHumanActionHub
}

func (h *HumanActionHub) WaitForAction(ctx context.Context, state domain.HandState, timeoutMS uint64) (domain.Action, error) {
	acting, ok := seatByNo(state.Seats, state.ActingSeat)
	if !ok {
		return domain.Action{}, fmt.Errorf("acting seat %d not found", state.ActingSeat)
	}
	toCall := uint32(0)
	if state.CurrentBet > acting.CommittedInRound {
		toCall = state.CurrentBet - acting.CommittedInRound
	}
	legalKinds := deriveLegalActionKinds(state, acting, toCall)
	legalSet := make(map[domain.ActionKind]struct{}, len(legalKinds))
	legalActions := make([]string, 0, len(legalKinds))
	for _, kind := range legalKinds {
		legalSet[kind] = struct{}{}
		legalActions = append(legalActions, string(kind))
	}
	var minRaiseTo *uint32
	if _, ok := legalSet[domain.ActionRaise]; ok {
		value := state.MinRaiseTo
		minRaiseTo = &value
	}

	holeCards, _ := holeCardsForSeat(state.HoleCards, state.ActingSeat)
	payload := HumanPendingAction{
		TableID:          state.TableID,
		HandID:           state.HandID,
		SeatNo:           int(state.ActingSeat),
		HoleCards:        cardsToASCII(holeCards),
		Board:            cardsToASCII(state.Board),
		Pot:              state.Pot,
		ToCall:           toCall,
		MinRaiseTo:       minRaiseTo,
		LegalActions:     legalActions,
		ActionDeadlineMS: timeoutMSOrDefault(timeoutMS),
	}

	key := pendingKey{tableID: state.TableID, seatNo: state.ActingSeat}
	out := make(chan domain.Action, 1)
	h.mu.Lock()
	h.pending[key] = pendingAction{
		payload: payload,
		legal:   legalSet,
		out:     out,
	}
	h.mu.Unlock()

	timer := time.NewTimer(time.Duration(timeoutMSOrDefault(timeoutMS)) * time.Millisecond)
	defer timer.Stop()

	select {
	case action := <-out:
		h.clearPending(key)
		return action, nil
	case <-timer.C:
		h.clearPending(key)
		return domain.Action{}, fmt.Errorf("human action timeout for table=%s seat=%d", state.TableID, state.ActingSeat)
	case <-ctx.Done():
		h.clearPending(key)
		return domain.Action{}, ctx.Err()
	}
}

func (h *HumanActionHub) SubmitAction(tableID string, req HumanActionRequest) error {
	seatNo, err := domain.NewSeatNo(uint8(req.SeatNo), domain.DefaultMaxSeats)
	if err != nil {
		return err
	}
	key := pendingKey{tableID: tableID, seatNo: seatNo}

	h.mu.Lock()
	pending, ok := h.pending[key]
	h.mu.Unlock()
	if !ok {
		return fmt.Errorf("no pending action for table=%s seat=%d", tableID, req.SeatNo)
	}

	kind := domain.ActionKind(strings.TrimSpace(req.Action))
	if _, ok := pending.legal[kind]; !ok {
		return fmt.Errorf("action %q is not legal for current pending turn", req.Action)
	}
	action, err := domain.NewAction(kind, req.Amount)
	if err != nil {
		return err
	}

	select {
	case pending.out <- action:
		return nil
	default:
		return fmt.Errorf("pending action channel is full for table=%s seat=%d", tableID, req.SeatNo)
	}
}

func (h *HumanActionHub) GetPending(tableID string, seatNo int) (HumanPendingAction, bool) {
	seat, err := domain.NewSeatNo(uint8(seatNo), domain.DefaultMaxSeats)
	if err != nil {
		return HumanPendingAction{}, false
	}
	key := pendingKey{tableID: tableID, seatNo: seat}
	h.mu.Lock()
	defer h.mu.Unlock()
	pending, ok := h.pending[key]
	if !ok {
		return HumanPendingAction{}, false
	}
	return pending.payload, true
}

func (h *HumanActionHub) clearPending(key pendingKey) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.pending, key)
}

func timeoutMSOrDefault(value uint64) uint64 {
	if value == 0 {
		return defaultHumanActionTimeoutMS
	}
	return value
}

func cardsToASCII(cards []domain.Card) []string {
	out := make([]string, 0, len(cards))
	for _, card := range cards {
		out = append(out, formatCardASCII(card))
	}
	return out
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

func deriveLegalActionKinds(state domain.HandState, acting domain.SeatState, toCall uint32) []domain.ActionKind {
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
