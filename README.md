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
