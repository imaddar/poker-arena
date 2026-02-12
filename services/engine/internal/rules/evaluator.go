package rules

import (
	"sort"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

type HandCategory uint8

const (
	HandCategoryHighCard HandCategory = iota + 1
	HandCategoryOnePair
	HandCategoryTwoPair
	HandCategoryThreeOfAKind
	HandCategoryStraight
	HandCategoryFlush
	HandCategoryFullHouse
	HandCategoryFourOfAKind
	HandCategoryStraightFlush
)

type HandRank struct {
	Category HandCategory
	Tiebreak []uint8
}

func EvaluateBestHand(hole []domain.Card, board []domain.Card) HandRank {
	all := append(append([]domain.Card(nil), hole...), board...)
	if len(all) < 5 {
		return HandRank{}
	}

	indices := combinations(len(all), 5)
	best := evaluateFiveCards([]domain.Card{all[indices[0][0]], all[indices[0][1]], all[indices[0][2]], all[indices[0][3]], all[indices[0][4]]})
	for _, c := range indices[1:] {
		candidate := evaluateFiveCards([]domain.Card{all[c[0]], all[c[1]], all[c[2]], all[c[3]], all[c[4]]})
		if CompareHandRank(candidate, best) > 0 {
			best = candidate
		}
	}
	return best
}

func CompareHandRank(a HandRank, b HandRank) int {
	if a.Category > b.Category {
		return 1
	}
	if a.Category < b.Category {
		return -1
	}

	n := len(a.Tiebreak)
	if len(b.Tiebreak) < n {
		n = len(b.Tiebreak)
	}
	for i := 0; i < n; i++ {
		if a.Tiebreak[i] > b.Tiebreak[i] {
			return 1
		}
		if a.Tiebreak[i] < b.Tiebreak[i] {
			return -1
		}
	}
	if len(a.Tiebreak) > len(b.Tiebreak) {
		return 1
	}
	if len(a.Tiebreak) < len(b.Tiebreak) {
		return -1
	}
	return 0
}

func evaluateFiveCards(cards []domain.Card) HandRank {
	ranks := make([]uint8, 0, 5)
	rankCounts := map[uint8]int{}
	suits := map[domain.Suit]int{}
	for _, card := range cards {
		r := uint8(card.Rank)
		ranks = append(ranks, r)
		rankCounts[r]++
		suits[card.Suit]++
	}
	sort.Slice(ranks, func(i, j int) bool { return ranks[i] > ranks[j] })

	isFlush := len(suits) == 1
	straightHigh, isStraight := straightHighRank(ranks)

	if isFlush && isStraight {
		return HandRank{Category: HandCategoryStraightFlush, Tiebreak: []uint8{straightHigh}}
	}

	groups := rankGroups(rankCounts)
	if groups[0].count == 4 {
		return HandRank{Category: HandCategoryFourOfAKind, Tiebreak: []uint8{groups[0].rank, groups[1].rank}}
	}
	if groups[0].count == 3 && groups[1].count == 2 {
		return HandRank{Category: HandCategoryFullHouse, Tiebreak: []uint8{groups[0].rank, groups[1].rank}}
	}
	if isFlush {
		return HandRank{Category: HandCategoryFlush, Tiebreak: ranks}
	}
	if isStraight {
		return HandRank{Category: HandCategoryStraight, Tiebreak: []uint8{straightHigh}}
	}
	if groups[0].count == 3 {
		tiebreak := []uint8{groups[0].rank}
		for _, g := range groups[1:] {
			tiebreak = append(tiebreak, g.rank)
		}
		return HandRank{Category: HandCategoryThreeOfAKind, Tiebreak: tiebreak}
	}
	if groups[0].count == 2 && groups[1].count == 2 {
		kicker := groups[2].rank
		highPair := groups[0].rank
		lowPair := groups[1].rank
		if lowPair > highPair {
			highPair, lowPair = lowPair, highPair
		}
		return HandRank{Category: HandCategoryTwoPair, Tiebreak: []uint8{highPair, lowPair, kicker}}
	}
	if groups[0].count == 2 {
		tiebreak := []uint8{groups[0].rank}
		for _, g := range groups[1:] {
			tiebreak = append(tiebreak, g.rank)
		}
		return HandRank{Category: HandCategoryOnePair, Tiebreak: tiebreak}
	}
	return HandRank{Category: HandCategoryHighCard, Tiebreak: ranks}
}

type rankGroup struct {
	rank  uint8
	count int
}

func rankGroups(rankCounts map[uint8]int) []rankGroup {
	groups := make([]rankGroup, 0, len(rankCounts))
	for rank, count := range rankCounts {
		groups = append(groups, rankGroup{rank: rank, count: count})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].count == groups[j].count {
			return groups[i].rank > groups[j].rank
		}
		return groups[i].count > groups[j].count
	})
	return groups
}

func straightHighRank(ranks []uint8) (uint8, bool) {
	unique := make([]uint8, 0, len(ranks))
	seen := map[uint8]struct{}{}
	for _, rank := range ranks {
		if _, ok := seen[rank]; ok {
			continue
		}
		seen[rank] = struct{}{}
		unique = append(unique, rank)
	}
	sort.Slice(unique, func(i, j int) bool { return unique[i] > unique[j] })
	if len(unique) != 5 {
		return 0, false
	}

	// Wheel straight: A-2-3-4-5.
	if unique[0] == 14 && unique[1] == 5 && unique[2] == 4 && unique[3] == 3 && unique[4] == 2 {
		return 5, true
	}

	for i := 1; i < 5; i++ {
		if unique[i-1]-1 != unique[i] {
			return 0, false
		}
	}
	return unique[0], true
}

func combinations(n int, choose int) [][]int {
	out := make([][]int, 0)
	combo := make([]int, choose)
	var walk func(start int, depth int)
	walk = func(start int, depth int) {
		if depth == choose {
			copied := append([]int(nil), combo...)
			out = append(out, copied)
			return
		}
		for i := start; i <= n-(choose-depth); i++ {
			combo[depth] = i
			walk(i+1, depth+1)
		}
	}
	walk(0, 0)
	return out
}
