package rules

import (
	"fmt"
	"sort"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

func ResolvePots(state domain.HandState) (domain.HandState, []domain.PotAward, error) {
	if len(state.Board) != 5 {
		return state, nil, fmt.Errorf("showdown requires 5 board cards, got %d", len(state.Board))
	}

	holeBySeat := map[domain.SeatNo][]domain.Card{}
	for _, seatCards := range state.HoleCards {
		holeBySeat[seatCards.SeatNo] = append([]domain.Card(nil), seatCards.Cards...)
	}

	levels := contributionLevels(state.Seats)
	if len(levels) == 0 {
		state.Pot = 0
		state.ShowdownAwards = nil
		state.Phase = domain.HandPhaseComplete
		return state, nil, nil
	}

	awards := make([]domain.PotAward, 0, len(levels))
	prev := uint32(0)
	for i, level := range levels {
		if level <= prev {
			continue
		}

		contributors := make([]int, 0, len(state.Seats))
		for seatIdx, seat := range state.Seats {
			if seat.TotalCommitted >= level {
				contributors = append(contributors, seatIdx)
			}
		}
		potAmount := (level - prev) * uint32(len(contributors))
		prev = level
		if potAmount == 0 {
			continue
		}

		winnerIdxs := make([]int, 0, len(contributors))
		bestRank := HandRank{}
		for _, seatIdx := range contributors {
			seat := state.Seats[seatIdx]
			if seat.Folded || seat.Status != domain.SeatStatusActive {
				continue
			}
			hole := holeBySeat[seat.SeatNo]
			if len(hole) != 2 {
				return state, nil, fmt.Errorf("seat %d missing hole cards", seat.SeatNo)
			}
			rank := EvaluateBestHand(hole, state.Board)
			if len(winnerIdxs) == 0 || CompareHandRank(rank, bestRank) > 0 {
				bestRank = rank
				winnerIdxs = []int{seatIdx}
				continue
			}
			if CompareHandRank(rank, bestRank) == 0 {
				winnerIdxs = append(winnerIdxs, seatIdx)
			}
		}
		if len(winnerIdxs) == 0 {
			continue
		}

		share := potAmount / uint32(len(winnerIdxs))
		odd := potAmount % uint32(len(winnerIdxs))
		for _, winner := range winnerIdxs {
			state.Seats[winner].Stack += share
		}

		orderedForOdd := orderWinnersForOddChip(state.ButtonSeat, winnerIdxs, state.Seats)
		for j := uint32(0); j < odd; j++ {
			state.Seats[orderedForOdd[j]].Stack++
		}

		winnerSeats := make([]domain.SeatNo, 0, len(winnerIdxs))
		for _, winner := range winnerIdxs {
			winnerSeats = append(winnerSeats, state.Seats[winner].SeatNo)
		}
		sort.Slice(winnerSeats, func(a, b int) bool { return winnerSeats[a] < winnerSeats[b] })

		reason := "main_pot"
		if i > 0 {
			reason = fmt.Sprintf("side_pot_%d", i)
		}
		awards = append(awards, domain.PotAward{Amount: potAmount, Seats: winnerSeats, Reason: reason})
	}

	state.Pot = 0
	state.ShowdownAwards = awards
	state.Phase = domain.HandPhaseComplete
	return state, awards, nil
}

func AwardUncontested(state domain.HandState) domain.HandState {
	if state.Pot == 0 {
		state.ShowdownAwards = nil
		state.Phase = domain.HandPhaseComplete
		return state
	}

	winnerIdx := -1
	for i, seat := range state.Seats {
		if seat.Status == domain.SeatStatusActive && !seat.Folded {
			winnerIdx = i
			break
		}
	}
	if winnerIdx == -1 {
		for i, seat := range state.Seats {
			if seat.Status == domain.SeatStatusActive {
				winnerIdx = i
				break
			}
		}
	}
	if winnerIdx == -1 {
		state.ShowdownAwards = nil
		state.Phase = domain.HandPhaseComplete
		return state
	}

	amount := state.Pot
	state.Seats[winnerIdx].Stack += amount
	state.Pot = 0
	state.ShowdownAwards = []domain.PotAward{{
		Amount: amount,
		Seats:  []domain.SeatNo{state.Seats[winnerIdx].SeatNo},
		Reason: "uncontested",
	}}
	state.Phase = domain.HandPhaseComplete
	return state
}

func contributionLevels(seats []domain.SeatState) []uint32 {
	seen := map[uint32]struct{}{}
	levels := make([]uint32, 0, len(seats))
	for _, seat := range seats {
		if seat.TotalCommitted == 0 {
			continue
		}
		if _, ok := seen[seat.TotalCommitted]; ok {
			continue
		}
		seen[seat.TotalCommitted] = struct{}{}
		levels = append(levels, seat.TotalCommitted)
	}
	sort.Slice(levels, func(i, j int) bool { return levels[i] < levels[j] })
	return levels
}

func orderWinnersForOddChip(button domain.SeatNo, winnerIdx []int, seats []domain.SeatState) []int {
	if len(winnerIdx) <= 1 {
		return winnerIdx
	}
	winnerBySeat := make(map[domain.SeatNo]int, len(winnerIdx))
	orderedSeats := make([]domain.SeatNo, 0, len(winnerIdx))
	for _, idx := range winnerIdx {
		seatNo := seats[idx].SeatNo
		winnerBySeat[seatNo] = idx
		orderedSeats = append(orderedSeats, seatNo)
	}
	sort.Slice(orderedSeats, func(i, j int) bool { return orderedSeats[i] < orderedSeats[j] })

	start := 0
	for i, seatNo := range orderedSeats {
		if seatNo > button {
			start = i
			break
		}
		if i == len(orderedSeats)-1 {
			start = 0
		}
	}

	ordered := make([]int, 0, len(winnerIdx))
	for i := 0; i < len(orderedSeats); i++ {
		seatNo := orderedSeats[(start+i)%len(orderedSeats)]
		ordered = append(ordered, winnerBySeat[seatNo])
	}
	return ordered
}
