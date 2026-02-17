# Agent Control-Plane Smoke Test

This smoke test verifies the secured control-plane + agent callback path end to end:

0. start a local Postgres instance,
1. start two mock agent HTTP endpoints,
2. start the control-plane server,
3. start a table run through the API,
4. verify status/hands/actions output,
5. stop the run if needed.

## Prerequisites

- Go installed and available in `PATH`
- `curl` installed
- Docker installed (for local Postgres quickstart)
- Repo root: `/Users/imaddar/git-repos/poker-arena`

## Terminal 0: Start Local Postgres

```bash
docker run --name poker-arena-postgres \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_DB=poker_arena \
  -p 5432:5432 \
  -d postgres:16
```

If the container already exists, use:

```bash
docker start poker-arena-postgres
```

## Terminal 1: Start Mock Agent A (port 9001)

```bash
cat >/tmp/mock-agent-a.go <<'EOF'
package main

import (
	"encoding/json"
	"log"
	"net/http"
)

type req struct {
	ToCall       uint32   `json:"to_call"`
	LegalActions []string `json:"legal_actions"`
}

func has(legal []string, want string) bool {
	for _, action := range legal {
		if action == want {
			return true
		}
	}
	return false
}

func main() {
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var body req
		_ = json.NewDecoder(r.Body).Decode(&body)

		action := "fold"
		if body.ToCall > 0 && has(body.LegalActions, "call") {
			action = "call"
		} else if body.ToCall == 0 && has(body.LegalActions, "check") {
			action = "check"
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"action": action,
		})
	})

	log.Println("mock agent A listening on :9001")
	log.Fatal(http.ListenAndServe(":9001", nil))
}
EOF

go run /tmp/mock-agent-a.go
```

## Terminal 2: Start Mock Agent B (port 9002)

```bash
cat >/tmp/mock-agent-b.go <<'EOF'
package main

import (
	"encoding/json"
	"log"
	"net/http"
)

type req struct {
	ToCall       uint32   `json:"to_call"`
	LegalActions []string `json:"legal_actions"`
}

func has(legal []string, want string) bool {
	for _, action := range legal {
		if action == want {
			return true
		}
	}
	return false
}

func main() {
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var body req
		_ = json.NewDecoder(r.Body).Decode(&body)

		action := "fold"
		if body.ToCall > 0 && has(body.LegalActions, "call") {
			action = "call"
		} else if body.ToCall == 0 && has(body.LegalActions, "check") {
			action = "check"
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"action": action,
		})
	})

	log.Println("mock agent B listening on :9002")
	log.Fatal(http.ListenAndServe(":9002", nil))
}
EOF

go run /tmp/mock-agent-b.go
```

## Terminal 3: Start Control Plane

```bash
cd /Users/imaddar/git-repos/poker-arena

export CONTROLPLANE_ADMIN_TOKENS="local-admin-token"
export CONTROLPLANE_SEAT_TOKENS="1:local-seat-1-token,2:local-seat-2-token"
export AGENT_ENDPOINT_ALLOWLIST="127.0.0.1:9001,127.0.0.1:9002"
export AGENT_HTTP_TIMEOUT_MS="2000"
export DATABASE_URL="postgres://postgres:postgres@127.0.0.1:5432/poker_arena?sslmode=disable"

./scripts/api-local.sh serve :8080
```

## Terminal 4: Run Smoke Flow

```bash
cd /Users/imaddar/git-repos/poker-arena

export BASE_URL="http://127.0.0.1:8080"
export TABLE_ID="smoke-table-1"
export ADMIN_API_TOKEN="local-admin-token"
export SEAT_API_TOKEN="local-seat-1-token"

# create resources
USER_ID="$(./scripts/api-local.sh create-user smoke-user smoke-user-token | jq -r '.ID')"
AGENT1_ID="$(./scripts/api-local.sh create-agent "${USER_ID}" smoke-agent-a | jq -r '.ID')"
AGENT2_ID="$(./scripts/api-local.sh create-agent "${USER_ID}" smoke-agent-b | jq -r '.ID')"
VERSION1_ID="$(./scripts/api-local.sh create-version "${AGENT1_ID}" http://127.0.0.1:9001/callback | jq -r '.ID')"
VERSION2_ID="$(./scripts/api-local.sh create-version "${AGENT2_ID}" http://127.0.0.1:9002/callback | jq -r '.ID')"
TABLE_ID="$(./scripts/api-local.sh create-table smoke-table 6 50 100 | jq -r '.ID')"
export TABLE_ID

# seat agents
./scripts/api-local.sh join 1 "${AGENT1_ID}" "${VERSION1_ID}" 10000 active
./scripts/api-local.sh join 2 "${AGENT2_ID}" "${VERSION2_ID}" 10000 active

# start 2 hands from persisted table seats
./scripts/api-local.sh start 2

# poll run status until it reaches completed
./scripts/api-local.sh status
sleep 1
./scripts/api-local.sh status

# inspect hand history
./scripts/api-local.sh hands

# copy first hand_id from the hands output and query actions
./scripts/api-local.sh actions <hand_id>

# fetch full replay payload for the same hand
./scripts/api-local.sh replay <hand_id>
```

Expected outcome:

- `status` eventually reports `completed`
- `hands` returns at least one hand
- `actions` returns betting actions for the hand

## Optional: Stop Early

If you started a longer run and want to terminate it:

```bash
./scripts/api-local.sh stop
./scripts/api-local.sh status
```

Expected status after stop: `stopped`.

## Cleanup

```bash
docker stop poker-arena-postgres
```
