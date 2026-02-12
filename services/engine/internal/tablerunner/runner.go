package tablerunner

import (
	"context"
	"errors"
	"fmt"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	"github.com/imaddar/poker-arena/services/engine/internal/rules"
	"github.com/imaddar/poker-arena/services/engine/internal/statemachine"
)

const defaultMaxActionsPerHand = 512

var (
	ErrActionLimitExceeded     = errors.New("action limit exceeded")
	ErrRunnerMisconfigured     = errors.New("runner misconfigured")
	ErrContextCancelled        = errors.New("runner context cancelled")
	ErrInvalidHandsToRun       = errors.New("hands to run must be greater than zero")
	ErrInsufficientActiveSeats = errors.New("insufficient active seats to start hand")
)

type ActionProvider interface {
	NextAction(ctx context.Context, state domain.HandState) (domain.Action, error)
}

type RunHandInput struct {
	TableID    string
	HandNo     uint64
	ButtonSeat domain.SeatNo
	Seats      []domain.SeatState
	Config     domain.TableConfig
}

type RunnerConfig struct {
	MaxActionsPerHand int
	OnHandComplete    func(HandSummary)
}

type Runner struct {
	provider ActionProvider
	config   RunnerConfig
}

type RunHandResult struct {
	FinalState    domain.HandState
	ActionCount   int
	FallbackCount int
}

type RunTableInput struct {
	TableID      string
	StartingHand uint64
	HandsToRun   int
	ButtonSeat   domain.SeatNo
	Seats        []domain.SeatState
	Config       domain.TableConfig
}

type HandSummary struct {
	HandNo        uint64
	FinalPhase    domain.HandPhase
	ActionCount   int
	FallbackCount int
	FinalState    domain.HandState
}

type RunTableResult struct {
	HandsCompleted int
	FinalButton    domain.SeatNo
	FinalSeats     []domain.SeatState
	TotalActions   int
	TotalFallbacks int
	HandSummaries  []HandSummary
}

func New(provider ActionProvider, config RunnerConfig) Runner {
	return Runner{
		provider: provider,
		config:   config,
	}
}

func (r Runner) RunTable(ctx context.Context, input RunTableInput) (RunTableResult, error) {
	var result RunTableResult

	if input.HandsToRun <= 0 {
		return result, ErrInvalidHandsToRun
	}
	if r.provider == nil {
		return result, ErrRunnerMisconfigured
	}
	if err := checkContext(ctx); err != nil {
		return result, err
	}

	seats := prepareSeatsForNextHand(input.Seats)
	button := input.ButtonSeat
	result.HandSummaries = make([]HandSummary, 0, input.HandsToRun)

	for i := 0; i < input.HandsToRun; i++ {
		if err := checkContext(ctx); err != nil {
			result.FinalButton = button
			result.FinalSeats = cloneSeats(seats)
			return result, err
		}

		if countActivePlayableSeats(seats) < int(input.Config.MinPlayersToStart) {
			result.FinalButton = button
			result.FinalSeats = cloneSeats(seats)
			return result, ErrInsufficientActiveSeats
		}

		currentButton, err := normalizeButton(button, seats)
		if err != nil {
			result.FinalButton = button
			result.FinalSeats = cloneSeats(seats)
			return result, err
		}

		handNo := input.StartingHand + uint64(i)
		handResult, err := r.RunHand(ctx, RunHandInput{
			TableID:    input.TableID,
			HandNo:     handNo,
			ButtonSeat: currentButton,
			Seats:      cloneSeats(seats),
			Config:     input.Config,
		})
		if err != nil {
			result.FinalButton = currentButton
			result.FinalSeats = cloneSeats(seats)
			return result, err
		}

		result.HandsCompleted++
		result.TotalActions += handResult.ActionCount
		result.TotalFallbacks += handResult.FallbackCount
		result.HandSummaries = append(result.HandSummaries, HandSummary{
			HandNo:        handNo,
			FinalPhase:    handResult.FinalState.Phase,
			ActionCount:   handResult.ActionCount,
			FallbackCount: handResult.FallbackCount,
			FinalState:    cloneHandState(handResult.FinalState),
		})
		if r.config.OnHandComplete != nil {
			r.config.OnHandComplete(result.HandSummaries[len(result.HandSummaries)-1])
		}

		seats = prepareSeatsForNextHand(handResult.FinalState.Seats)
		nextButton, err := nextButtonSeat(currentButton, seats)
		if err != nil {
			result.FinalButton = currentButton
			result.FinalSeats = cloneSeats(seats)
			return result, err
		}
		button = nextButton
	}

	result.FinalButton = button
	result.FinalSeats = cloneSeats(seats)
	return result, nil
}

func (r Runner) RunHand(ctx context.Context, input RunHandInput) (RunHandResult, error) {
	var result RunHandResult

	if r.provider == nil {
		return result, ErrRunnerMisconfigured
	}

	maxActions := r.config.MaxActionsPerHand
	if maxActions <= 0 {
		maxActions = defaultMaxActionsPerHand
	}

	state, err := statemachine.StartNewHand(statemachine.StartNewHandInput{
		TableID:    input.TableID,
		HandNo:     input.HandNo,
		Seats:      input.Seats,
		ButtonSeat: input.ButtonSeat,
		Config:     input.Config,
	})
	if err != nil {
		return result, err
	}
	result.FinalState = state

	for {
		if isTerminal(state) {
			if state.Phase == domain.HandPhaseShowdown {
				resolved, _, err := rules.ResolvePots(state)
				if err != nil {
					result.FinalState = state
					return result, err
				}
				state = resolved
			}
			result.FinalState = state
			return result, nil
		}

		if err := checkContext(ctx); err != nil {
			result.FinalState = state
			return result, err
		}

		action, err := r.provider.NextAction(ctx, state)
		if err != nil {
			if err := checkContext(ctx); err != nil {
				result.FinalState = state
				return result, err
			}

			state, err = r.applyFallback(state)
			if err != nil {
				result.FinalState = state
				return result, fmt.Errorf("apply fallback after provider error: %w", err)
			}

			result.ActionCount++
			result.FallbackCount++
			result.FinalState = state

			if result.ActionCount > maxActions {
				return result, fmt.Errorf("%w: applied %d actions (max %d)", ErrActionLimitExceeded, result.ActionCount, maxActions)
			}
			continue
		}

		if err := checkContext(ctx); err != nil {
			result.FinalState = state
			return result, err
		}

		nextState, err := statemachine.ApplyAction(state, action)
		if err != nil {
			if err := checkContext(ctx); err != nil {
				result.FinalState = state
				return result, err
			}

			state, err = r.applyFallback(state)
			if err != nil {
				result.FinalState = state
				return result, fmt.Errorf("apply fallback after illegal action: %w", err)
			}

			result.ActionCount++
			result.FallbackCount++
			result.FinalState = state

			if result.ActionCount > maxActions {
				return result, fmt.Errorf("%w: applied %d actions (max %d)", ErrActionLimitExceeded, result.ActionCount, maxActions)
			}
			continue
		}

		state = nextState
		result.ActionCount++
		result.FinalState = state

		if result.ActionCount > maxActions {
			return result, fmt.Errorf("%w: applied %d actions (max %d)", ErrActionLimitExceeded, result.ActionCount, maxActions)
		}
	}
}

func (r Runner) applyFallback(state domain.HandState) (domain.HandState, error) {
	checkAction := fallbackActionCheck()
	nextState, err := statemachine.ApplyAction(state, checkAction)
	if err == nil {
		return nextState, nil
	}

	foldAction := fallbackActionFold()
	nextState, foldErr := statemachine.ApplyAction(state, foldAction)
	if foldErr != nil {
		return state, fmt.Errorf("fallback check failed (%v) and fallback fold failed (%w)", err, foldErr)
	}

	return nextState, nil
}

func fallbackActionCheck() domain.Action {
	action, _ := domain.NewAction(domain.ActionCheck, nil)
	return action
}

func fallbackActionFold() domain.Action {
	action, _ := domain.NewAction(domain.ActionFold, nil)
	return action
}

func checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", ErrContextCancelled, ctx.Err())
	default:
		return nil
	}
}

func isTerminal(state domain.HandState) bool {
	return state.Phase == domain.HandPhaseComplete || state.Phase == domain.HandPhaseShowdown
}

func prepareSeatsForNextHand(seats []domain.SeatState) []domain.SeatState {
	prepared := cloneSeats(seats)
	for i := range prepared {
		prepared[i].TotalCommitted = 0
		prepared[i].CommittedInRound = 0
		prepared[i].HasActedThisRound = false
		prepared[i].Folded = false
		if prepared[i].Stack == 0 {
			prepared[i].Status = domain.SeatStatusBusted
		}
	}
	return prepared
}

func countActivePlayableSeats(seats []domain.SeatState) int {
	count := 0
	for _, seat := range seats {
		if seat.Status == domain.SeatStatusActive && seat.Stack > 0 {
			count++
		}
	}
	return count
}

func normalizeButton(current domain.SeatNo, seats []domain.SeatState) (domain.SeatNo, error) {
	if isButtonEligible(current, seats) {
		return current, nil
	}
	return nextButtonSeat(current, seats)
}

func nextButtonSeat(current domain.SeatNo, seats []domain.SeatState) (domain.SeatNo, error) {
	if len(seats) == 0 {
		return 0, ErrInsufficientActiveSeats
	}

	ordered := cloneSeats(seats)
	sortSeatsBySeatNo(ordered)
	start := 0
	found := false
	for i, seat := range ordered {
		if seat.SeatNo == current {
			start = i
			found = true
			break
		}
	}
	if !found {
		return 0, ErrInsufficientActiveSeats
	}

	for i := 1; i <= len(ordered); i++ {
		candidate := ordered[(start+i)%len(ordered)]
		if candidate.Status != domain.SeatStatusBusted && candidate.Stack > 0 {
			return candidate.SeatNo, nil
		}
	}

	return 0, ErrInsufficientActiveSeats
}

func isButtonEligible(seatNo domain.SeatNo, seats []domain.SeatState) bool {
	for _, seat := range seats {
		if seat.SeatNo == seatNo && seat.Status != domain.SeatStatusBusted && seat.Stack > 0 {
			return true
		}
	}
	return false
}

func cloneSeats(seats []domain.SeatState) []domain.SeatState {
	return append([]domain.SeatState(nil), seats...)
}

func cloneHandState(state domain.HandState) domain.HandState {
	cloned := state
	cloned.Seats = cloneSeats(state.Seats)
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

func sortSeatsBySeatNo(seats []domain.SeatState) {
	for i := 0; i < len(seats); i++ {
		for j := i + 1; j < len(seats); j++ {
			if seats[j].SeatNo < seats[i].SeatNo {
				seats[i], seats[j] = seats[j], seats[i]
			}
		}
	}
}
