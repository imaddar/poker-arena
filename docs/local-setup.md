# Local Setup Guide

This guide is the fastest path to get the full stack running locally and verified.

## 1) Prerequisites

- `go` (for control-plane service)
- `node` + `npm` (for frontend)
- `docker` (for local Postgres)
- `curl` and `jq` (used by smoke scripts)

Repo root assumed in this guide:

```bash
cd /Users/imaddar/git-repos/poker-arena
```

## 2) Start Postgres

Option A (compose from repo):

```bash
docker compose -f infra/docker-compose.yml up -d postgres
```

Option B (if compose path is problematic in your current directory):

```bash
docker run --name poker-arena-postgres \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_DB=poker_arena \
  -p 5432:5432 \
  -d postgres:16
```

## 3) Start Control Plane

Use a port that is free. This guide uses `8081`.

```bash
cd /Users/imaddar/git-repos/poker-arena

export CONTROLPLANE_ADMIN_TOKENS=local-admin-token
export CONTROLPLANE_SEAT_TOKENS=1:local-seat-1-token,2:local-seat-2-token
export AGENT_ENDPOINT_ALLOWLIST=127.0.0.1:9001,127.0.0.1:9002
export CONTROLPLANE_CORS_ALLOWED_ORIGINS=http://localhost:5173
export DATABASE_URL=postgres://postgres:postgres@127.0.0.1:5432/poker_arena?sslmode=disable

go -C services/engine run ./cmd/controlplane -addr :8081
```

## 4) Seed a Table (new terminal)

```bash
cd /Users/imaddar/git-repos/poker-arena
BASE_URL=http://127.0.0.1:8081 \
ADMIN_API_TOKEN=local-admin-token \
./scripts/api-local.sh create-table smoke 6 50 100
```

## 5) Configure + Start Frontend

```bash
cd /Users/imaddar/git-repos/poker-arena
cat > frontend/web/.env.local <<'EOF'
VITE_USE_MOCK_API=false
VITE_API_BASE_URL=http://127.0.0.1:8081
VITE_ADMIN_TOKEN=local-admin-token
EOF

npm --prefix frontend/web run dev
```

Login options in the UI:

- Admin control-plane session token: `local-admin-token`
- Observer seat session token: `local-seat-1-token`

## 6) Run Automated Checks

Backend auth/role checks:

```bash
BASE_URL=http://127.0.0.1:8081 \
ADMIN_TOKEN=local-admin-token \
SEAT_TOKEN=local-seat-1-token \
./scripts/test-backend-auth-local.sh
```

Frontend logic + build:

```bash
./scripts/test-frontend-local.sh
```

Full local validation (backend + frontend + web smoke):

```bash
BASE_URL=http://127.0.0.1:8081 \
ADMIN_TOKEN=local-admin-token \
SEAT_TOKEN=local-seat-1-token \
./scripts/test-all-local.sh
```

## 7) Manual API Spot Checks

Admin session:

```bash
curl -sS -H "Authorization: Bearer local-admin-token" \
  "http://127.0.0.1:8081/session"
```

Seat session:

```bash
curl -sS -H "Authorization: Bearer local-seat-1-token" \
  "http://127.0.0.1:8081/session"
```

Latest table replay:

```bash
curl -sS -H "Authorization: Bearer local-admin-token" \
  "http://127.0.0.1:8081/tables/<table_id>/replay/latest"
```

## Troubleshooting

`port 5432 already allocated`:

- You already have a Postgres container bound to 5432.
- Use existing container (`docker start poker-arena-postgres`) or stop the conflicting one before compose.

`listen tcp :8080: bind: address already in use`:

- Another control-plane process is already running on 8080.
- Check with:
```bash
lsof -nP -iTCP:8080 -sTCP:LISTEN
```
- Either stop that process or run control-plane on another port (for example `:8081`).

`route not found` for `/tables/:id/replay/latest`:

- Restart control-plane from the latest local commit.

`table not found`:

- Replace `<table_id>` with a real ID from `GET /tables` or create one using `./scripts/api-local.sh create-table ...`.

## Expected Success Signal

When everything is healthy, this command should end with:

```text
[test-all] PASS: all local checks completed
```
