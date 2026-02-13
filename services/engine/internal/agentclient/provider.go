package agentclient

import (
	"context"
	"fmt"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

type SeatEndpointProvider interface {
	EndpointForSeat(state domain.HandState, seat domain.SeatNo) (string, error)
}

type ActionProvider struct {
	Client           Client
	Endpoints        SeatEndpointProvider
	DefaultTimeoutMS uint64
}

func (p ActionProvider) NextAction(ctx context.Context, state domain.HandState) (domain.Action, error) {
	if p.Endpoints == nil {
		return domain.Action{}, ErrEndpointNotConfigured
	}

	endpoint, err := p.Endpoints.EndpointForSeat(state, state.ActingSeat)
	if err != nil {
		return domain.Action{}, fmt.Errorf("%w: %v", ErrEndpointNotConfigured, err)
	}
	if endpoint == "" {
		return domain.Action{}, fmt.Errorf("%w: seat %d", ErrEndpointNotConfigured, state.ActingSeat)
	}

	timeoutMS := p.DefaultTimeoutMS
	if timeoutMS == 0 {
		timeoutMS = defaultActionTimeout
	}

	client := p.Client
	if client.httpClient == nil {
		client = New(defaultTimeout)
	}

	return client.NextAction(ctx, Request{
		EndpointURL:     endpoint,
		State:           state,
		ActingSeat:      state.ActingSeat,
		ActionTimeoutMS: timeoutMS,
	})
}
