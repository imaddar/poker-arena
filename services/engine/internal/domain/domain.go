package domain

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	DefaultMaxSeats          uint8  = 6
	DefaultMinPlayersToStart uint8  = 2
	DefaultStartingStack     uint32 = 10_000
	DefaultSmallBlind        uint32 = 50
	DefaultBigBlind          uint32 = 100
	DefaultActionTimeoutMS   uint64 = 2_000
)

var (
	ErrInvalidMinPlayersToStart = errors.New("min players to start must be at least 2 and <= max seats")
	ErrInvalidBlindStructure    = errors.New("big blind must be greater than or equal to small blind")
	ErrDuplicateSeat            = errors.New("duplicate seat numbers are not allowed")
)

type Suit string

const (
	SuitClubs    Suit = "clubs"
	SuitDiamonds Suit = "diamonds"
	SuitHearts   Suit = "hearts"
	SuitSpades   Suit = "spades"
)

type Rank uint8

func NewRank(value uint8) (Rank, error) {
	if value < 2 || value > 14 {
		return 0, fmt.Errorf("rank must be in range 2..=14, got %d", value)
	}
	return Rank(value), nil
}

type Card struct {
	Rank Rank `json:"rank"`
	Suit Suit `json:"suit"`
}

func NewCard(rank Rank, suit Suit) Card {
	return Card{Rank: rank, Suit: suit}
}

type Deck struct {
	Cards []Card `json:"cards"`
}

func Standard52Deck() Deck {
	cards := make([]Card, 0, 52)
	suits := []Suit{SuitClubs, SuitDiamonds, SuitHearts, SuitSpades}

	for _, suit := range suits {
		for rank := uint8(2); rank <= 14; rank++ {
			cards = append(cards, NewCard(Rank(rank), suit))
		}
	}

	return Deck{Cards: cards}
}

type Street string

const (
	StreetPreflop Street = "preflop"
	StreetFlop    Street = "flop"
	StreetTurn    Street = "turn"
	StreetRiver   Street = "river"
)

type ActionKind string

const (
	ActionFold  ActionKind = "fold"
	ActionCheck ActionKind = "check"
	ActionCall  ActionKind = "call"
	ActionBet   ActionKind = "bet"
	ActionRaise ActionKind = "raise"
)

type Action struct {
	Kind   ActionKind `json:"kind"`
	Amount *uint32    `json:"amount,omitempty"`
}

func NewAction(kind ActionKind, amount *uint32) (Action, error) {
	needsAmount := kind == ActionBet || kind == ActionRaise

	if needsAmount && amount == nil {
		return Action{}, fmt.Errorf("action amount is required for %s", kind)
	}

	if !needsAmount && amount != nil {
		return Action{}, fmt.Errorf("action amount is not allowed for %s", kind)
	}

	return Action{Kind: kind, Amount: amount}, nil
}

type SeatNo uint8

func NewSeatNo(value uint8, maxSeats uint8) (SeatNo, error) {
	if value == 0 || value > maxSeats {
		return 0, fmt.Errorf("seat number must be in range 1..=%d, got %d", maxSeats, value)
	}
	return SeatNo(value), nil
}

type SeatStatus string

const (
	SeatStatusActive     SeatStatus = "active"
	SeatStatusSittingOut SeatStatus = "sitting_out"
	SeatStatusBusted     SeatStatus = "busted"
)

type SeatState struct {
	SeatNo           SeatNo     `json:"seat_no"`
	Stack            uint32     `json:"stack"`
	CommittedInRound uint32     `json:"committed_in_round"`
	Status           SeatStatus `json:"status"`
}

func NewSeatState(seatNo SeatNo, stack uint32) SeatState {
	return SeatState{
		SeatNo:           seatNo,
		Stack:            stack,
		CommittedInRound: 0,
		Status:           SeatStatusActive,
	}
}

func (s SeatState) IsActive() bool {
	return s.Status == SeatStatusActive
}

type TableConfig struct {
	MaxSeats          uint8  `json:"max_seats"`
	MinPlayersToStart uint8  `json:"min_players_to_start"`
	StartingStack     uint32 `json:"starting_stack"`
	SmallBlind        uint32 `json:"small_blind"`
	BigBlind          uint32 `json:"big_blind"`
	ActionTimeoutMS   uint64 `json:"action_timeout_ms"`
}

func DefaultV0TableConfig() TableConfig {
	return TableConfig{
		MaxSeats:          DefaultMaxSeats,
		MinPlayersToStart: DefaultMinPlayersToStart,
		StartingStack:     DefaultStartingStack,
		SmallBlind:        DefaultSmallBlind,
		BigBlind:          DefaultBigBlind,
		ActionTimeoutMS:   DefaultActionTimeoutMS,
	}
}

func (c TableConfig) Validate() error {
	if c.MaxSeats < 2 || c.MaxSeats > DefaultMaxSeats {
		return fmt.Errorf("table max_seats must be in range 2..=%d, got %d", DefaultMaxSeats, c.MaxSeats)
	}

	if c.MinPlayersToStart < 2 || c.MinPlayersToStart > c.MaxSeats {
		return ErrInvalidMinPlayersToStart
	}

	if c.BigBlind < c.SmallBlind {
		return ErrInvalidBlindStructure
	}

	return nil
}

type HandPhase string

const (
	HandPhaseDealing  HandPhase = "dealing"
	HandPhaseBetting  HandPhase = "betting"
	HandPhaseShowdown HandPhase = "showdown"
	HandPhaseComplete HandPhase = "complete"
)

type HandState struct {
	HandID     string      `json:"hand_id"`
	TableID    string      `json:"table_id"`
	HandNo     uint64      `json:"hand_no"`
	ButtonSeat SeatNo      `json:"button_seat"`
	ActingSeat SeatNo      `json:"acting_seat"`
	Phase      HandPhase   `json:"phase"`
	Street     Street      `json:"street"`
	Pot        uint32      `json:"pot"`
	Board      []Card      `json:"board"`
	Seats      []SeatState `json:"seats"`
}

func NewHandState(
	tableID string,
	handNo uint64,
	buttonSeat SeatNo,
	actingSeat SeatNo,
	seats []SeatState,
	config TableConfig,
) (HandState, error) {
	if err := config.Validate(); err != nil {
		return HandState{}, err
	}

	activeCount := 0
	seenSeats := make(map[SeatNo]struct{}, len(seats))
	for _, seat := range seats {
		if seat.IsActive() {
			activeCount++
		}
		if _, exists := seenSeats[seat.SeatNo]; exists {
			return HandState{}, ErrDuplicateSeat
		}
		seenSeats[seat.SeatNo] = struct{}{}
	}

	if activeCount < int(config.MinPlayersToStart) {
		return HandState{}, fmt.Errorf("hand must start with at least %d active seats, got %d", config.MinPlayersToStart, activeCount)
	}

	if len(seats) > int(config.MaxSeats) {
		return HandState{}, fmt.Errorf("hand cannot exceed max seats (%d), got %d", config.MaxSeats, len(seats))
	}

	if _, ok := seenSeats[buttonSeat]; !ok {
		return HandState{}, fmt.Errorf("button seat %d must exist in hand seats", buttonSeat)
	}

	if _, ok := seenSeats[actingSeat]; !ok {
		return HandState{}, fmt.Errorf("acting seat %d must exist in hand seats", actingSeat)
	}

	handID, err := newHandID()
	if err != nil {
		return HandState{}, err
	}

	return HandState{
		HandID:     handID,
		TableID:    tableID,
		HandNo:     handNo,
		ButtonSeat: buttonSeat,
		ActingSeat: actingSeat,
		Phase:      HandPhaseDealing,
		Street:     StreetPreflop,
		Pot:        0,
		Board:      make([]Card, 0, 5),
		Seats:      append([]SeatState(nil), seats...),
	}, nil
}

func newHandID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate hand id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
