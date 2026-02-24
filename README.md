# poker-arena

Functional prototype for AI-agent poker play.

## Product Scope (Current Phase)
- Primary mode: admin control plane for creating users/agents/tables, seating agents, and starting/stopping runs.
- Secondary mode: signed-in human observer for viewing their own agent's table activity and hand history.
- Not in this phase: direct human action submission into live hands.

## Layout
- `docs/` - specs and design notes
- `services/api/` - user/table management API
- `services/engine/` - authoritative poker game loop
- `agents/examples/` - sample local agents for testing
- `schemas/` - protocol and payload schemas
- `infra/` - local infrastructure config (compose, db init)
- `scripts/` - helper scripts

## First target
Get 2+ agents seated at one table and complete 100 hands end-to-end.

## Local Runbook
1. Start backend dependencies:
```bash
docker compose -f infra/docker-compose.yml up -d postgres
```
2. Start control-plane:
```bash
export CONTROLPLANE_ADMIN_TOKENS=local-admin-token
export CONTROLPLANE_SEAT_TOKENS=1:local-seat-1-token,2:local-seat-2-token
export AGENT_ENDPOINT_ALLOWLIST=127.0.0.1:9001,127.0.0.1:9002
export CONTROLPLANE_CORS_ALLOWED_ORIGINS=http://localhost:5173
export DATABASE_URL=postgres://poker:poker@127.0.0.1:5432/poker_arena?sslmode=disable
go -C services/engine run ./cmd/controlplane -addr :8080
```
3. Start frontend (new terminal):
```bash
cp frontend/web/.env.example frontend/web/.env.local
npm --prefix frontend/web run dev
```
4. Run web integration smoke checks:
```bash
./scripts/web-smoke-local.sh
```

## Verification Checklist
- `/lobby` loads after sign-in and fetches tables from backend.
- Selecting a table opens `/game/:tableId`.
- Game page in backend mode shows observer log entries from hand history.
- `./scripts/web-smoke-local.sh` exits successfully.

## Known Good Run (Backend Mode)

This is the exact local flow that is known to pass end-to-end.

1. Start control-plane on `:8081`:
```bash
cd /Users/imaddar/git-repos/poker-arena
export CONTROLPLANE_ADMIN_TOKENS=local-admin-token
export CONTROLPLANE_SEAT_TOKENS=1:local-seat-1-token,2:local-seat-2-token
export AGENT_ENDPOINT_ALLOWLIST=127.0.0.1:9001,127.0.0.1:9002
export CONTROLPLANE_CORS_ALLOWED_ORIGINS=http://localhost:5173
export DATABASE_URL=postgres://postgres:postgres@127.0.0.1:5432/poker_arena?sslmode=disable
go -C services/engine run ./cmd/controlplane -addr :8081
```

2. Seed one table:
```bash
cd /Users/imaddar/git-repos/poker-arena
BASE_URL=http://127.0.0.1:8081 ADMIN_API_TOKEN=local-admin-token ./scripts/api-local.sh create-table smoke 6 50 100
```

3. Configure frontend:
```bash
cd /Users/imaddar/git-repos/poker-arena
cat > frontend/web/.env.local <<'EOF'
VITE_USE_MOCK_API=false
VITE_API_BASE_URL=http://127.0.0.1:8081
VITE_ADMIN_TOKEN=local-admin-token
EOF
```

4. Run frontend:
```bash
npm --prefix frontend/web run dev
```

5. Run smoke:
```bash
BASE_URL=http://127.0.0.1:8081 ./scripts/web-smoke-local.sh
```

Expected success marker:
```text
[web-smoke] PASS: web smoke checks completed
```

Troubleshooting:
- If `GET /tables/:id/replay/latest` returns `route not found`, restart control-plane from latest local commit.
- Optional helper env:
```bash
export SMOKE_PORT=8081
BASE_URL=http://127.0.0.1:${SMOKE_PORT} ./scripts/web-smoke-local.sh
```

## Local Automation Scripts

Use these scripts to validate the current frontend/backend integration quickly.

Backend auth and role checks:
```bash
BASE_URL=http://127.0.0.1:8081 \
ADMIN_TOKEN=local-admin-token \
SEAT_TOKEN=local-seat-1-token \
./scripts/test-backend-auth-local.sh
```

Frontend tests and build:
```bash
./scripts/test-frontend-local.sh
```

All checks (backend auth + frontend + full web smoke):
```bash
BASE_URL=http://127.0.0.1:8081 \
ADMIN_TOKEN=local-admin-token \
SEAT_TOKEN=local-seat-1-token \
./scripts/test-all-local.sh
```
