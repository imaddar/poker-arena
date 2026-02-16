package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"github.com/imaddar/poker-arena/services/engine/internal/api"
	"github.com/imaddar/poker-arena/services/engine/internal/domain"
	"github.com/imaddar/poker-arena/services/engine/internal/persistence"
	"github.com/imaddar/poker-arena/services/engine/internal/tablerunner"
	_ "github.com/lib/pq"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	bearerToken := strings.TrimSpace(os.Getenv("CONTROLPLANE_BEARER_TOKEN"))
	if bearerToken == "" {
		fmt.Fprintln(os.Stderr, "missing required env CONTROLPLANE_BEARER_TOKEN")
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
		AuthBearerToken:       bearerToken,
		AllowedAgentHosts:     allowlist,
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
