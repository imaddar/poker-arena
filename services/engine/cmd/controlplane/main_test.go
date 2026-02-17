package main

import (
	"testing"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

func TestParseAdminTokens_Valid(t *testing.T) {
	t.Parallel()

	tokens, err := parseAdminTokens("admin-a, admin-b")
	if err != nil {
		t.Fatalf("parseAdminTokens failed: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}
	if _, ok := tokens["admin-a"]; !ok {
		t.Fatal("expected admin-a token")
	}
}

func TestParseAdminTokens_RejectsEmptyToken(t *testing.T) {
	t.Parallel()

	if _, err := parseAdminTokens("admin-a,"); err == nil {
		t.Fatal("expected parseAdminTokens to fail for empty token")
	}
}

func TestParseSeatTokens_Valid(t *testing.T) {
	t.Parallel()

	tokens, err := parseSeatTokens("1:seat-a,2:seat-b", domain.DefaultMaxSeats)
	if err != nil {
		t.Fatalf("parseSeatTokens failed: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 seat tokens, got %d", len(tokens))
	}
	if tokens["seat-a"] != 1 {
		t.Fatalf("expected seat-a to map to seat 1, got %d", tokens["seat-a"])
	}
}

func TestParseSeatTokens_RejectsMalformedEntry(t *testing.T) {
	t.Parallel()

	if _, err := parseSeatTokens("abc", domain.DefaultMaxSeats); err == nil {
		t.Fatal("expected parseSeatTokens to fail for malformed entry")
	}
}

func TestParseSeatTokens_RejectsInvalidSeatNumber(t *testing.T) {
	t.Parallel()

	if _, err := parseSeatTokens("0:seat-a", domain.DefaultMaxSeats); err == nil {
		t.Fatal("expected parseSeatTokens to fail for invalid seat number")
	}
}

func TestParseSeatTokens_RejectsDuplicateSeatMapping(t *testing.T) {
	t.Parallel()

	if _, err := parseSeatTokens("1:seat-a,1:seat-b", domain.DefaultMaxSeats); err == nil {
		t.Fatal("expected parseSeatTokens to fail for duplicate seat mapping")
	}
}

func TestParseSeatTokens_RejectsDuplicateToken(t *testing.T) {
	t.Parallel()

	if _, err := parseSeatTokens("1:seat-a,2:seat-a", domain.DefaultMaxSeats); err == nil {
		t.Fatal("expected parseSeatTokens to fail for duplicate token")
	}
}

func TestHasTokenOverlap(t *testing.T) {
	t.Parallel()

	admin := map[string]struct{}{"token-a": {}}
	seat := map[string]domain.SeatNo{"token-b": 1}
	if hasTokenOverlap(admin, seat) {
		t.Fatal("did not expect overlap")
	}
	seat["token-a"] = 2
	if !hasTokenOverlap(admin, seat) {
		t.Fatal("expected overlap")
	}
}

func TestParseCORSAllowedOrigins(t *testing.T) {
	t.Parallel()

	origins := parseCORSAllowedOrigins("http://localhost:5173, https://arena.example.com")
	if len(origins) != 2 {
		t.Fatalf("expected 2 origins, got %d", len(origins))
	}
	if _, ok := origins["http://localhost:5173"]; !ok {
		t.Fatal("expected localhost origin in parsed set")
	}
	if _, ok := origins["https://arena.example.com"]; !ok {
		t.Fatal("expected production origin in parsed set")
	}
}

func TestParseCORSAllowedOrigins_EmptyInput(t *testing.T) {
	t.Parallel()

	origins := parseCORSAllowedOrigins("  ,   ")
	if len(origins) != 0 {
		t.Fatalf("expected empty origin set, got %d", len(origins))
	}
}
