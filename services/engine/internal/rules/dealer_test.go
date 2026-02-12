package rules

import (
	"reflect"
	"testing"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

func TestStandard52DeckHasUniqueCards(t *testing.T) {
	t.Parallel()

	deck := domain.Standard52Deck().Cards
	if len(deck) != 52 {
		t.Fatalf("expected 52 cards, got %d", len(deck))
	}

	seen := map[string]struct{}{}
	for _, card := range deck {
		key := cardKey(card)
		if _, ok := seen[key]; ok {
			t.Fatalf("duplicate card %s", key)
		}
		seen[key] = struct{}{}
	}
}

func TestSeededShuffleIsDeterministic(t *testing.T) {
	t.Parallel()

	deckA := append([]domain.Card(nil), domain.Standard52Deck().Cards...)
	deckB := append([]domain.Card(nil), domain.Standard52Deck().Cards...)

	if err := NewSeededShuffler(7).Shuffle(deckA); err != nil {
		t.Fatalf("shuffle A failed: %v", err)
	}
	if err := NewSeededShuffler(7).Shuffle(deckB); err != nil {
		t.Fatalf("shuffle B failed: %v", err)
	}

	if !reflect.DeepEqual(deckA, deckB) {
		t.Fatalf("expected identical shuffled decks for same seed")
	}
}

func TestSeededShuffleDiffersBySeed(t *testing.T) {
	t.Parallel()

	deckA := append([]domain.Card(nil), domain.Standard52Deck().Cards...)
	deckB := append([]domain.Card(nil), domain.Standard52Deck().Cards...)

	if err := NewSeededShuffler(7).Shuffle(deckA); err != nil {
		t.Fatalf("shuffle A failed: %v", err)
	}
	if err := NewSeededShuffler(11).Shuffle(deckB); err != nil {
		t.Fatalf("shuffle B failed: %v", err)
	}

	if reflect.DeepEqual(deckA, deckB) {
		t.Fatal("expected shuffled decks to differ for different seeds")
	}
}

func cardKey(card domain.Card) string {
	return string(card.Suit) + "-" + string(rune(card.Rank))
}
