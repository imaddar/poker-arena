package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"github.com/imaddar/poker-arena/services/engine/internal/api"
	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	"github.com/imaddar/poker-arena/services/engine/internal/persistence"
	"github.com/imaddar/poker-arena/services/engine/internal/tablerunner"
	_ "github.com/lib/pq"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	adminTokensRaw := strings.TrimSpace(os.Getenv("CONTROLPLANE_ADMIN_TOKENS"))
	if adminTokensRaw == "" {
		fmt.Fprintln(os.Stderr, "missing required env CONTROLPLANE_ADMIN_TOKENS")
		os.Exit(1)
	}
	adminTokens, err := parseAdminTokens(adminTokensRaw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid CONTROLPLANE_ADMIN_TOKENS: %v\n", err)
		os.Exit(1)
	}
	seatTokens, err := parseSeatTokens(strings.TrimSpace(os.Getenv("CONTROLPLANE_SEAT_TOKENS")), domain.DefaultMaxSeats)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid CONTROLPLANE_SEAT_TOKENS: %v\n", err)
		os.Exit(1)
	}
	if hasTokenOverlap(adminTokens, seatTokens) {
		fmt.Fprintln(os.Stderr, "invalid token config: a token cannot be both admin and seat scoped")
		os.Exit(1)
	}

	allowlistRaw := strings.TrimSpace(os.Getenv("AGENT_ENDPOINT_ALLOWLIST"))
	if allowlistRaw == "" {
		fmt.Fprintln(os.Stderr, "missing required env AGENT_ENDPOINT_ALLOWLIST")
		os.Exit(1)
	}
	allowlist := parseAllowlist(allowlistRaw)
	if len(allowlist) == 0 {
		fmt.Fprintln(os.Stderr, "AGENT_ENDPOINT_ALLOWLIST must include at least one host[:port]")
		os.Exit(1)
	}
	corsAllowedOrigins := parseCORSAllowedOrigins(strings.TrimSpace(os.Getenv("CONTROLPLANE_CORS_ALLOWED_ORIGINS")))

	httpTimeoutMS := uint64(domain.DefaultActionTimeoutMS)
	if raw := strings.TrimSpace(os.Getenv("AGENT_HTTP_TIMEOUT_MS")); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || parsed == 0 {
			fmt.Fprintf(os.Stderr, "invalid AGENT_HTTP_TIMEOUT_MS value %q\n", raw)
			os.Exit(1)
		}
		httpTimeoutMS = parsed
	}

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "missing required env DATABASE_URL")
		os.Exit(1)
	}
	if !hasSQLDriver("postgres") {
		fmt.Fprintln(os.Stderr, "postgres SQL driver is not linked; add a driver import such as github.com/lib/pq in this binary")
		os.Exit(1)
	}

	maxOpenConns := parsePositiveIntEnvOrDefault("DATABASE_MAX_OPEN_CONNS", 10)
	maxIdleConns := parsePositiveIntEnvOrDefault("DATABASE_MAX_IDLE_CONNS", 5)
	connMaxLifetimeSec := parsePositiveIntEnvOrDefault("DATABASE_CONN_MAX_LIFETIME_SEC", 300)

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(time.Duration(connMaxLifetimeSec) * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "database ping failed: %v\n", err)
		os.Exit(1)
	}
	if err := persistence.MigratePostgres(ctx, db); err != nil {
		fmt.Fprintf(os.Stderr, "database migration failed: %v\n", err)
		os.Exit(1)
	}

	repo := persistence.NewPostgresRepository(db)
	serverConfig := api.ServerConfig{
		AdminBearerTokens:     adminTokens,
		SeatBearerTokens:      seatTokens,
		AllowedAgentHosts:     allowlist,
		AllowedCORSOrigins:    corsAllowedOrigins,
		DefaultAgentTimeoutMS: httpTimeoutMS,
		AgentHTTPTimeout:      time.Duration(httpTimeoutMS) * time.Millisecond,
	}

	server := api.NewServer(
		repo,
		func(provider tablerunner.ActionProvider, cfg tablerunner.RunnerConfig) api.Runner {
			return tablerunner.New(provider, cfg)
		},
		newProviderFactory(serverConfig.AgentHTTPTimeout),
		serverConfig,
	)

	fmt.Fprintf(os.Stdout, "engine control-plane listening on %s\n", *addr)
	if err := http.ListenAndServe(*addr, server); err != nil {
		fmt.Fprintf(os.Stderr, "server failed: %v\n", err)
		os.Exit(1)
	}
}

func parsePositiveIntEnvOrDefault(key string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		fmt.Fprintf(os.Stderr, "invalid %s value %q\n", key, raw)
		os.Exit(1)
	}
	return value
}

func hasSQLDriver(name string) bool {
	for _, driver := range sql.Drivers() {
		if driver == name {
			return true
		}
	}
	return false
}

func parseAllowlist(raw string) map[string]struct{} {
	hosts := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		host := strings.TrimSpace(part)
		if host == "" {
			continue
		}
		hosts[host] = struct{}{}
	}
	return hosts
}

func parseCORSAllowedOrigins(raw string) map[string]struct{} {
	origins := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(part)
		if origin == "" {
			continue
		}
		origins[origin] = struct{}{}
	}
	return origins
}

func parseAdminTokens(raw string) (map[string]struct{}, error) {
	tokens := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		token := strings.TrimSpace(part)
		if token == "" {
			return nil, errors.New("admin token list contains an empty token")
		}
		tokens[token] = struct{}{}
	}
	if len(tokens) == 0 {
		return nil, errors.New("at least one admin token is required")
	}
	return tokens, nil
}

func parseSeatTokens(raw string, maxSeats uint8) (map[string]domain.SeatNo, error) {
	tokens := make(map[string]domain.SeatNo)
	if raw == "" {
		return tokens, nil
	}

	usedSeats := make(map[domain.SeatNo]struct{})
	for _, part := range strings.Split(raw, ",") {
		entry := strings.TrimSpace(part)
		if entry == "" {
			return nil, errors.New("seat token list contains an empty entry")
		}
		pieces := strings.SplitN(entry, ":", 2)
		if len(pieces) != 2 {
			return nil, fmt.Errorf("expected seat_no:token entry, got %q", entry)
		}
		seatRaw := strings.TrimSpace(pieces[0])
		token := strings.TrimSpace(pieces[1])
		if token == "" {
			return nil, fmt.Errorf("seat token entry %q has empty token", entry)
		}
		seatUint64, err := strconv.ParseUint(seatRaw, 10, 8)
		if err != nil {
			return nil, fmt.Errorf("seat token entry %q has invalid seat number", entry)
		}
		seatNo, err := domain.NewSeatNo(uint8(seatUint64), maxSeats)
		if err != nil {
			return nil, err
		}
		if _, exists := usedSeats[seatNo]; exists {
			return nil, fmt.Errorf("duplicate seat mapping for seat %d", seatNo)
		}
		usedSeats[seatNo] = struct{}{}
		if _, exists := tokens[token]; exists {
			return nil, fmt.Errorf("duplicate token %q in seat token map", token)
		}
		tokens[token] = seatNo
	}
	return tokens, nil
}

func hasTokenOverlap(adminTokens map[string]struct{}, seatTokens map[string]domain.SeatNo) bool {
	for token := range seatTokens {
		if _, exists := adminTokens[token]; exists {
			return true
		}
	}
	return false
}
