package rules

import (
	cryptorand "crypto/rand"
	"fmt"
	"math/big"
	"math/rand"
	"sort"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

type Shuffler interface {
	Shuffle([]domain.Card) error
}

type Dealer interface {
	InitHand(state domain.HandState) (domain.HandState, error)
	DealPreflop(state domain.HandState) (domain.HandState, error)
	DealFlopTurnRiver(state domain.HandState) (domain.HandState, error)
}

type cryptoShuffler struct{}

type seededShuffler struct {
	rng *rand.Rand
}

type standardDealer struct {
	shuffler Shuffler
}

func NewCryptoShuffler() Shuffler {
	return cryptoShuffler{}
}

func NewSeededShuffler(seed int64) Shuffler {
	return seededShuffler{rng: rand.New(rand.NewSource(seed))}
}

func NewDealer(shuffler Shuffler) Dealer {
	if shuffler == nil {
		shuffler = NewCryptoShuffler()
	}
	return standardDealer{shuffler: shuffler}
}

func (s cryptoShuffler) Shuffle(cards []domain.Card) error {
	for i := len(cards) - 1; i > 0; i-- {
		n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return fmt.Errorf("crypto shuffle failed: %w", err)
		}
		j := int(n.Int64())
		cards[i], cards[j] = cards[j], cards[i]
	}
	return nil
}

func (s seededShuffler) Shuffle(cards []domain.Card) error {
	for i := len(cards) - 1; i > 0; i-- {
		j := s.rng.Intn(i + 1)
		cards[i], cards[j] = cards[j], cards[i]
	}
	return nil
}

func (d standardDealer) InitHand(state domain.HandState) (domain.HandState, error) {
	deck := domain.Standard52Deck().Cards
	if err := d.shuffler.Shuffle(deck); err != nil {
		return state, err
	}
	state.Deck = deck
	state.NextCardIndex = 0
	state.Board = make([]domain.Card, 0, 5)
	state.HoleCards = make([]domain.SeatCards, 0, len(state.Seats))
	state.ShowdownAwards = make([]domain.PotAward, 0, 4)
	return state, nil
}

func (d standardDealer) DealPreflop(state domain.HandState) (domain.HandState, error) {
	if len(state.Deck) != 52 {
		return state, fmt.Errorf("cannot deal preflop: deck size is %d", len(state.Deck))
	}

	ordered := activeSeatsInDealOrder(state.Seats, state.ButtonSeat)
	if len(ordered) < 2 {
		return state, fmt.Errorf("cannot deal preflop: active seats=%d", len(ordered))
	}

	hole := make(map[domain.SeatNo][]domain.Card, len(ordered))
	for round := 0; round < 2; round++ {
		for _, seatNo := range ordered {
			card, err := drawCard(&state)
			if err != nil {
				return state, err
			}
			hole[seatNo] = append(hole[seatNo], card)
		}
	}

	state.HoleCards = make([]domain.SeatCards, 0, len(hole))
	for _, seatNo := range ordered {
		state.HoleCards = append(state.HoleCards, domain.SeatCards{
			SeatNo: seatNo,
			Cards:  append([]domain.Card(nil), hole[seatNo]...),
		})
	}
	return state, nil
}

func (d standardDealer) DealFlopTurnRiver(state domain.HandState) (domain.HandState, error) {
	if len(state.Deck) != 52 {
		return state, fmt.Errorf("cannot deal board: deck size is %d", len(state.Deck))
	}

	// Burn one card before every post-flop street.
	if _, err := drawCard(&state); err != nil {
		return state, err
	}

	draw := 0
	switch state.Street {
	case domain.StreetPreflop:
		draw = 3
	case domain.StreetFlop, domain.StreetTurn:
		draw = 1
	default:
		return state, fmt.Errorf("cannot deal next street from %s", state.Street)
	}

	for i := 0; i < draw; i++ {
		card, err := drawCard(&state)
		if err != nil {
			return state, err
		}
		state.Board = append(state.Board, card)
	}

	return state, nil
}

func activeSeatsInDealOrder(seats []domain.SeatState, button domain.SeatNo) []domain.SeatNo {
	ordered := make([]domain.SeatNo, 0, len(seats))
	for _, seat := range seats {
		if seat.Status == domain.SeatStatusActive && seat.Stack > 0 {
			ordered = append(ordered, seat.SeatNo)
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	if len(ordered) == 0 {
		return ordered
	}

	buttonIdx := 0
	for i, seatNo := range ordered {
		if seatNo == button {
			buttonIdx = i
			break
		}
	}

	out := make([]domain.SeatNo, 0, len(ordered))
	for i := 1; i <= len(ordered); i++ {
		out = append(out, ordered[(buttonIdx+i)%len(ordered)])
	}
	return out
}

func drawCard(state *domain.HandState) (domain.Card, error) {
	if state.NextCardIndex >= len(state.Deck) {
		return domain.Card{}, fmt.Errorf("deck exhausted at card index %d", state.NextCardIndex)
	}
	card := state.Deck[state.NextCardIndex]
	state.NextCardIndex++
	return card, nil
}
