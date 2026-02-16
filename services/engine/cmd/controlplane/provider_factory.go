package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/imaddar/poker-arena/services/engine/internal/agentclient"
	"github.com/imaddar/poker-arena/services/engine/internal/api"
	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	"github.com/imaddar/poker-arena/services/engine/internal/tablerunner"
)

func newProviderFactory(clientTimeout time.Duration) func(tableID string, start api.StartRequest, cfg api.ServerConfig) (tablerunner.ActionProvider, error) {
	return func(_ string, start api.StartRequest, cfg api.ServerConfig) (tablerunner.ActionProvider, error) {
		maxSeats := domain.DefaultV0TableConfig().MaxSeats
		if start.TableConfig != nil {
			maxSeats = start.TableConfig.MaxSeats
		}

		endpoints := make(map[domain.SeatNo]string, len(start.Seats))
		seatTimeouts := make(map[domain.SeatNo]uint64, len(start.Seats))
		for _, seat := range start.Seats {
			seatNo, err := domain.NewSeatNo(seat.SeatNo, maxSeats)
			if err != nil {
				return nil, err
			}
			if isSeatActive(seat.Status) {
				endpoint := strings.TrimSpace(seat.AgentEndpoint)
				if endpoint == "" {
					return nil, fmt.Errorf("%w: seat %d", agentclient.ErrEndpointNotConfigured, seatNo)
				}
				endpoints[seatNo] = endpoint
			}
			if seat.AgentTimeoutMS != nil && *seat.AgentTimeoutMS > 0 {
				seatTimeouts[seatNo] = *seat.AgentTimeoutMS
			}
		}

		defaultTimeout := cfg.DefaultAgentTimeoutMS
		if defaultTimeout == 0 {
			defaultTimeout = domain.DefaultActionTimeoutMS
		}

		return seatTimeoutProvider{
			client:         agentclient.New(clientTimeout),
			endpointLookup: tableSeatEndpointProvider{endpoints: endpoints},
			defaultTimeout: defaultTimeout,
			seatTimeouts:   seatTimeouts,
		}, nil
	}
}

type seatTimeoutProvider struct {
	client         agentclient.Client
	endpointLookup tableSeatEndpointProvider
	defaultTimeout uint64
	seatTimeouts   map[domain.SeatNo]uint64
}

func (p seatTimeoutProvider) NextAction(ctx context.Context, state domain.HandState) (domain.Action, error) {
	endpoint, err := p.endpointLookup.EndpointForSeat(state, state.ActingSeat)
	if err != nil {
		return domain.Action{}, err
	}

	timeout := p.defaultTimeout
	if value, ok := p.seatTimeouts[state.ActingSeat]; ok {
		timeout = value
	}

	return p.client.NextAction(ctx, agentclient.Request{
		EndpointURL:     endpoint,
		State:           state,
		ActingSeat:      state.ActingSeat,
		ActionTimeoutMS: timeout,
	})
}

type tableSeatEndpointProvider struct {
	endpoints map[domain.SeatNo]string
}

func (p tableSeatEndpointProvider) EndpointForSeat(_ domain.HandState, seat domain.SeatNo) (string, error) {
	endpoint, ok := p.endpoints[seat]
	if !ok || strings.TrimSpace(endpoint) == "" {
		return "", fmt.Errorf("%w: seat %d", agentclient.ErrEndpointNotConfigured, seat)
	}
	return endpoint, nil
}

func isSeatActive(status domain.SeatStatus) bool {
	return status == "" || status == domain.SeatStatusActive
}
