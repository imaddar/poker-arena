package tablerunner

import (
	"context"
	"errors"
	"fmt"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	"github.com/imaddar/poker-arena/services/engine/internal/statemachine"
)

const defaultMaxActionsPerHand = 512

var (
	ErrActionLimitExceeded = errors.New("action limit exceeded")
	ErrRunnerMisconfigured = errors.New("runner misconfigured")
	ErrContextCancelled    = errors.New("runner context cancelled")
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

func New(provider ActionProvider, config RunnerConfig) Runner {
	return Runner{
		provider: provider,
		config:   config,
	}
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
