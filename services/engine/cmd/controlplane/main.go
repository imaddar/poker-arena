package main

import (
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

	repo := persistence.NewInMemoryRepository()
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
