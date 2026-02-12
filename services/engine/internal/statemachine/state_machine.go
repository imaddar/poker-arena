package statemachine

import (
	"errors"
	"fmt"
	"sort"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	"github.com/imaddar/poker-arena/services/engine/internal/rules"
)

var (
	ErrIllegalAction       = errors.New("illegal action")
	ErrNotActingSeat       = errors.New("action does not match acting seat")
	ErrInsufficientChips   = errors.New("insufficient chips for action")
	ErrHandAlreadyComplete = errors.New("hand already complete")
	ErrNoActiveSeats       = errors.New("hand has no active seats")
	ErrInvalidTransition   = errors.New("invalid hand transition")
)

type StartNewHandInput struct {
	TableID    string
	HandNo     uint64
	Seats      []domain.SeatState
	ButtonSeat domain.SeatNo
	Config     domain.TableConfig
	Shuffler   rules.Shuffler
}

func StartNewHand(input StartNewHandInput) (domain.HandState, error) {
	seats := append([]domain.SeatState(nil), input.Seats...)
	sortSeats(seats)

	if countNonFoldedActiveSeats(seats) == 0 {
		return domain.HandState{}, ErrNoActiveSeats
	}

	sbSeat, ok := nextSeat(seats, input.ButtonSeat, false, isActiveSeat)
	if !ok {
		return domain.HandState{}, ErrNoActiveSeats
	}
	bbSeat, ok := nextSeat(seats, sbSeat, false, isActiveSeat)
	if !ok {
		return domain.HandState{}, ErrNoActiveSeats
	}
	actingSeat, ok := nextSeat(seats, bbSeat, false, isEligibleToAct)
	if !ok {
		// If every active seat is all-in after blinds there is no one to act.
		actingSeat = sbSeat
	}

	state, err := domain.NewHandState(
		input.TableID,
		input.HandNo,
		input.ButtonSeat,
		actingSeat,
		seats,
		input.Config,
	)
	if err != nil {
		return domain.HandState{}, err
	}

	dealer := rules.NewDealer(input.Shuffler)
	state, err = dealer.InitHand(state)
	if err != nil {
		return domain.HandState{}, err
	}
	state, err = dealer.DealPreflop(state)
	if err != nil {
		return domain.HandState{}, err
	}

	state.Phase = domain.HandPhaseBetting
	state.ActionOrderStartSeat = actingSeat
	postSB := postBlind(&state, sbSeat, input.Config.SmallBlind)
	postBB := postBlind(&state, bbSeat, input.Config.BigBlind)
	state.CurrentBet = postBB
	state.LastFullRaise = input.Config.BigBlind
	state.MinRaiseTo = state.CurrentBet + state.LastFullRaise
	bb := bbSeat
	state.LastAggressorSeat = &bb

	if postSB == 0 && postBB == 0 {
		return domain.HandState{}, fmt.Errorf("%w: failed to post blinds", ErrInvalidTransition)
	}

	if countNonFoldedActiveSeats(state.Seats) <= 1 {
		state = rules.AwardUncontested(state)
		return state, nil
	}

	if nextActor, ok := nextSeat(state.Seats, bbSeat, false, isEligibleToAct); ok {
		state.ActingSeat = nextActor
		state.ActionOrderStartSeat = nextActor
		return state, nil
	}

	state.Phase = domain.HandPhaseShowdown
	return state, nil
}

func ApplyAction(state domain.HandState, action domain.Action) (domain.HandState, error) {
	next := cloneState(state)

	if next.Phase == domain.HandPhaseComplete || next.Phase == domain.HandPhaseShowdown {
		return domain.HandState{}, ErrHandAlreadyComplete
	}
	if next.Phase != domain.HandPhaseBetting {
		return domain.HandState{}, ErrInvalidTransition
	}

	actingIdx := seatIndex(next.Seats, next.ActingSeat)
	if actingIdx < 0 {
		return domain.HandState{}, ErrInvalidTransition
	}
	if !isEligibleToAct(next.Seats[actingIdx]) {
		return domain.HandState{}, ErrNotActingSeat
	}

	toCall := computeToCall(next.Seats[actingIdx], next.CurrentBet)

	switch action.Kind {
	case domain.ActionFold:
		next.Seats[actingIdx].Folded = true
		next.Seats[actingIdx].HasActedThisRound = true
	case domain.ActionCheck:
		if toCall != 0 {
			return domain.HandState{}, ErrIllegalAction
		}
		next.Seats[actingIdx].HasActedThisRound = true
	case domain.ActionCall:
		if toCall == 0 {
			return domain.HandState{}, ErrIllegalAction
		}
		pay := min(toCall, next.Seats[actingIdx].Stack)
		next.Seats[actingIdx].Stack -= pay
		next.Seats[actingIdx].TotalCommitted += pay
		next.Seats[actingIdx].CommittedInRound += pay
		next.Seats[actingIdx].HasActedThisRound = true
		next.Pot += pay
	case domain.ActionBet:
		if next.CurrentBet != 0 || action.Amount == nil || *action.Amount == 0 {
			return domain.HandState{}, ErrIllegalAction
		}
		if *action.Amount > next.Seats[actingIdx].Stack {
			return domain.HandState{}, ErrInsufficientChips
		}
		amount := *action.Amount
		next.Seats[actingIdx].Stack -= amount
		next.Seats[actingIdx].TotalCommitted += amount
		next.Seats[actingIdx].CommittedInRound += amount
		next.Pot += amount
		next.CurrentBet = next.Seats[actingIdx].CommittedInRound
		next.LastFullRaise = amount
		next.MinRaiseTo = next.CurrentBet + next.LastFullRaise
		markRoundResponsePending(next.Seats, actingIdx)
		seat := next.Seats[actingIdx].SeatNo
		next.LastAggressorSeat = &seat
	case domain.ActionRaise:
		if next.CurrentBet == 0 || action.Amount == nil {
			return domain.HandState{}, ErrIllegalAction
		}
		raiseTo := *action.Amount
		if raiseTo <= next.CurrentBet || raiseTo < next.MinRaiseTo {
			return domain.HandState{}, ErrIllegalAction
		}
		if raiseTo <= next.Seats[actingIdx].CommittedInRound {
			return domain.HandState{}, ErrIllegalAction
		}
		delta := raiseTo - next.Seats[actingIdx].CommittedInRound
		if delta > next.Seats[actingIdx].Stack {
			return domain.HandState{}, ErrInsufficientChips
		}
		previousBet := next.CurrentBet
		next.Seats[actingIdx].Stack -= delta
		next.Seats[actingIdx].TotalCommitted += delta
		next.Seats[actingIdx].CommittedInRound += delta
		next.Pot += delta
		next.CurrentBet = raiseTo
		next.LastFullRaise = raiseTo - previousBet
		next.MinRaiseTo = next.CurrentBet + next.LastFullRaise
		markRoundResponsePending(next.Seats, actingIdx)
		seat := next.Seats[actingIdx].SeatNo
		next.LastAggressorSeat = &seat
	default:
		return domain.HandState{}, ErrIllegalAction
	}

	if countNonFoldedActiveSeats(next.Seats) <= 1 {
		next = rules.AwardUncontested(next)
		return next, nil
	}

	if isBettingRoundClosed(next) {
		if err := advanceStreet(&next); err != nil {
			return domain.HandState{}, err
		}
		if next.Phase != domain.HandPhaseBetting {
			return next, nil
		}
		return next, nil
	}

	nextActor, ok := nextSeat(next.Seats, next.Seats[actingIdx].SeatNo, false, isEligibleToAct)
	if !ok {
		if err := advanceStreet(&next); err != nil {
			return domain.HandState{}, err
		}
		return next, nil
	}
	next.ActingSeat = nextActor
	return next, nil
}

func postBlind(state *domain.HandState, seatNo domain.SeatNo, amount uint32) uint32 {
	idx := seatIndex(state.Seats, seatNo)
	if idx < 0 || !isActiveSeat(state.Seats[idx]) {
		return 0
	}
	post := min(state.Seats[idx].Stack, amount)
	state.Seats[idx].Stack -= post
	state.Seats[idx].TotalCommitted += post
	state.Seats[idx].CommittedInRound += post
	state.Pot += post
	return post
}

func isBettingRoundClosed(state domain.HandState) bool {
	if countEligibleToActSeats(state.Seats) <= 1 {
		return true
	}

	for _, seat := range state.Seats {
		if !isEligibleToAct(seat) {
			continue
		}
		if !seat.HasActedThisRound {
			return false
		}
		if state.CurrentBet > 0 && seat.CommittedInRound != state.CurrentBet {
			return false
		}
	}

	return true
}

func advanceStreet(state *domain.HandState) error {
	for i := range state.Seats {
		state.Seats[i].CommittedInRound = 0
		state.Seats[i].HasActedThisRound = false
	}
	state.CurrentBet = 0
	state.LastAggressorSeat = nil
	state.LastFullRaise = state.BigBlind
	state.MinRaiseTo = state.BigBlind

	switch state.Street {
	case domain.StreetPreflop:
		dealt, err := rules.NewDealer(nil).DealFlopTurnRiver(*state)
		if err != nil {
			return err
		}
		*state = dealt
		state.Street = domain.StreetFlop
	case domain.StreetFlop:
		dealt, err := rules.NewDealer(nil).DealFlopTurnRiver(*state)
		if err != nil {
			return err
		}
		*state = dealt
		state.Street = domain.StreetTurn
	case domain.StreetTurn:
		dealt, err := rules.NewDealer(nil).DealFlopTurnRiver(*state)
		if err != nil {
			return err
		}
		*state = dealt
		state.Street = domain.StreetRiver
	case domain.StreetRiver:
		state.Phase = domain.HandPhaseShowdown
		return nil
	default:
		state.Phase = domain.HandPhaseShowdown
		return nil
	}

	start, ok := nextSeat(state.Seats, state.ButtonSeat, false, isEligibleToAct)
	if !ok {
		if countNonFoldedActiveSeats(state.Seats) <= 1 {
			*state = rules.AwardUncontested(*state)
		} else {
			state.Phase = domain.HandPhaseShowdown
		}
		return nil
	}
	state.ActingSeat = start
	state.ActionOrderStartSeat = start
	state.Phase = domain.HandPhaseBetting
	return nil
}

func sortSeats(seats []domain.SeatState) {
	sort.Slice(seats, func(i, j int) bool {
		return seats[i].SeatNo < seats[j].SeatNo
	})
}

func seatIndex(seats []domain.SeatState, seatNo domain.SeatNo) int {
	for i, seat := range seats {
		if seat.SeatNo == seatNo {
			return i
		}
	}
	return -1
}

func nextSeat(
	seats []domain.SeatState,
	from domain.SeatNo,
	includeFrom bool,
	filter func(domain.SeatState) bool,
) (domain.SeatNo, bool) {
	if len(seats) == 0 {
		return 0, false
	}
	ordered := append([]domain.SeatState(nil), seats...)
	sortSeats(ordered)

	start := 0
	found := false
	for i, seat := range ordered {
		if seat.SeatNo == from {
			start = i
			found = true
			break
		}
	}
	if !found {
		return 0, false
	}

	offset := 1
	if includeFrom {
		offset = 0
	}
	for i := 0; i < len(ordered); i++ {
		seat := ordered[(start+offset+i)%len(ordered)]
		if filter(seat) {
			return seat.SeatNo, true
		}
	}
	return 0, false
}

func isActiveSeat(seat domain.SeatState) bool {
	return seat.IsActive()
}

func isEligibleToAct(seat domain.SeatState) bool {
	return seat.IsActive() && seat.Stack > 0
}

func countNonFoldedActiveSeats(seats []domain.SeatState) int {
	count := 0
	for _, seat := range seats {
		if seat.IsActive() {
			count++
		}
	}
	return count
}

func countEligibleToActSeats(seats []domain.SeatState) int {
	count := 0
	for _, seat := range seats {
		if isEligibleToAct(seat) {
			count++
		}
	}
	return count
}

func computeToCall(seat domain.SeatState, currentBet uint32) uint32 {
	if currentBet <= seat.CommittedInRound {
		return 0
	}
	return currentBet - seat.CommittedInRound
}

func markRoundResponsePending(seats []domain.SeatState, aggressorIdx int) {
	for i := range seats {
		seats[i].HasActedThisRound = false
	}
	seats[aggressorIdx].HasActedThisRound = true
}

func cloneState(state domain.HandState) domain.HandState {
	cloned := state
	cloned.Seats = append([]domain.SeatState(nil), state.Seats...)
	cloned.Board = append([]domain.Card(nil), state.Board...)
	cloned.Deck = append([]domain.Card(nil), state.Deck...)
	cloned.HoleCards = make([]domain.SeatCards, 0, len(state.HoleCards))
	for _, seatCards := range state.HoleCards {
		cloned.HoleCards = append(cloned.HoleCards, domain.SeatCards{
			SeatNo: seatCards.SeatNo,
			Cards:  append([]domain.Card(nil), seatCards.Cards...),
		})
	}
	cloned.ShowdownAwards = make([]domain.PotAward, 0, len(state.ShowdownAwards))
	for _, award := range state.ShowdownAwards {
		cloned.ShowdownAwards = append(cloned.ShowdownAwards, domain.PotAward{
			Amount: award.Amount,
			Seats:  append([]domain.SeatNo(nil), award.Seats...),
			Reason: award.Reason,
		})
	}
	if state.LastAggressorSeat != nil {
		seat := *state.LastAggressorSeat
		cloned.LastAggressorSeat = &seat
	}
	return cloned
}

func min(a uint32, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}
