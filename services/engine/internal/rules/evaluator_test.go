package rules

import (
	"testing"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

func TestEvaluateBestHand_AllCategories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		hole     []domain.Card
		board    []domain.Card
		category HandCategory
	}{
		{
			name:     "high card",
			hole:     cards(t, "As", "7d"),
			board:    cards(t, "2c", "4h", "9s", "Jd", "3c"),
			category: HandCategoryHighCard,
		},
		{
			name:     "one pair",
			hole:     cards(t, "As", "Ad"),
			board:    cards(t, "2c", "4h", "9s", "Jd", "3c"),
			category: HandCategoryOnePair,
		},
		{
			name:     "two pair",
			hole:     cards(t, "As", "Ad"),
			board:    cards(t, "2c", "2h", "9s", "Jd", "3c"),
			category: HandCategoryTwoPair,
		},
		{
			name:     "trips",
			hole:     cards(t, "As", "Ad"),
			board:    cards(t, "Ac", "2h", "9s", "Jd", "3c"),
			category: HandCategoryThreeOfAKind,
		},
		{
			name:     "straight",
			hole:     cards(t, "8s", "7d"),
			board:    cards(t, "6c", "5h", "4s", "Kd", "3c"),
			category: HandCategoryStraight,
		},
		{
			name:     "flush",
			hole:     cards(t, "As", "7s"),
			board:    cards(t, "2s", "4s", "9s", "Jd", "3c"),
			category: HandCategoryFlush,
		},
		{
			name:     "full house",
			hole:     cards(t, "As", "Ad"),
			board:    cards(t, "Ac", "2h", "2s", "Jd", "3c"),
			category: HandCategoryFullHouse,
		},
		{
			name:     "quads",
			hole:     cards(t, "As", "Ad"),
			board:    cards(t, "Ac", "Ah", "2s", "Jd", "3c"),
			category: HandCategoryFourOfAKind,
		},
		{
			name:     "straight flush",
			hole:     cards(t, "8s", "7s"),
			board:    cards(t, "6s", "5s", "4s", "Jd", "3c"),
			category: HandCategoryStraightFlush,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			rank := EvaluateBestHand(tc.hole, tc.board)
			if rank.Category != tc.category {
				t.Fatalf("expected category %d, got %d", tc.category, rank.Category)
			}
		})
	}
}

func TestEvaluateBestHand_AceLowStraight(t *testing.T) {
	t.Parallel()

	rank := EvaluateBestHand(cards(t, "As", "2d"), cards(t, "3c", "4h", "5s", "Kd", "Qc"))
	if rank.Category != HandCategoryStraight {
		t.Fatalf("expected straight, got %d", rank.Category)
	}
	if rank.Tiebreak[0] != 5 {
		t.Fatalf("expected wheel straight high card 5, got %d", rank.Tiebreak[0])
	}
}

func TestEvaluateBestHand_BroadwayStraight(t *testing.T) {
	t.Parallel()

	rank := EvaluateBestHand(cards(t, "As", "Kd"), cards(t, "Qc", "Jh", "Ts", "2d", "3c"))
	if rank.Category != HandCategoryStraight {
		t.Fatalf("expected straight, got %d", rank.Category)
	}
	if rank.Tiebreak[0] != 14 {
		t.Fatalf("expected broadway straight high card 14, got %d", rank.Tiebreak[0])
	}
}

func TestCompareHandRank_TieBreakers(t *testing.T) {
	t.Parallel()

	betterTwoPair := EvaluateBestHand(cards(t, "As", "Ad"), cards(t, "Ks", "Kh", "2c", "9d", "3c"))
	worseTwoPair := EvaluateBestHand(cards(t, "Qs", "Qd"), cards(t, "Js", "Jh", "2c", "9d", "3c"))
	if CompareHandRank(betterTwoPair, worseTwoPair) <= 0 {
		t.Fatal("expected AA+KK to beat QQ+JJ")
	}

	betterKicker := EvaluateBestHand(cards(t, "As", "Kd"), cards(t, "Ac", "7h", "4s", "3d", "2c"))
	worseKicker := EvaluateBestHand(cards(t, "As", "Qd"), cards(t, "Ac", "7h", "4s", "3d", "2c"))
	if CompareHandRank(betterKicker, worseKicker) <= 0 {
		t.Fatal("expected AK pair hand to beat AQ pair hand")
	}

	betterFullHouse := EvaluateBestHand(cards(t, "As", "Ad"), cards(t, "Ac", "Kh", "Ks", "2d", "3c"))
	worseFullHouse := EvaluateBestHand(cards(t, "Qs", "Qd"), cards(t, "Qc", "Ah", "As", "2d", "3c"))
	if CompareHandRank(betterFullHouse, worseFullHouse) <= 0 {
		t.Fatal("expected aces full to beat queens full")
	}
}

func TestCompareHandRank_BoardPlayedTie(t *testing.T) {
	t.Parallel()

	board := cards(t, "As", "Ks", "Qs", "Js", "Ts")
	rankA := EvaluateBestHand(cards(t, "2c", "3d"), board)
	rankB := EvaluateBestHand(cards(t, "4c", "5d"), board)
	if CompareHandRank(rankA, rankB) != 0 {
		t.Fatal("expected exact tie when board is royal flush")
	}
}

func cards(t *testing.T, values ...string) []domain.Card {
	t.Helper()
	out := make([]domain.Card, 0, len(values))
	for _, v := range values {
		out = append(out, mustCard(t, v))
	}
	return out
}

func mustCard(t *testing.T, value string) domain.Card {
	t.Helper()
	if len(value) != 2 {
		t.Fatalf("invalid card format %q", value)
	}

	var rank uint8
	switch value[0] {
	case 'A':
		rank = 14
	case 'K':
		rank = 13
	case 'Q':
		rank = 12
	case 'J':
		rank = 11
	case 'T':
		rank = 10
	default:
		rank = value[0] - '0'
	}

	r, err := domain.NewRank(rank)
	if err != nil {
		t.Fatalf("invalid rank in %q: %v", value, err)
	}

	var suit domain.Suit
	switch value[1] {
	case 'c':
		suit = domain.SuitClubs
	case 'd':
		suit = domain.SuitDiamonds
	case 'h':
		suit = domain.SuitHearts
	case 's':
		suit = domain.SuitSpades
	default:
		t.Fatalf("invalid suit in %q", value)
	}

	return domain.NewCard(r, suit)
}
