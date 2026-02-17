# Production Readiness Gap (MVP -> Production)

## Purpose
This document explicitly lists what is acceptable for MVP speed and what must be completed before production launch.

## MVP Acceptable Now
- Single-tenant admin-driven control plane operation.
- Static bearer token maps for admin and seat-scoped replay access.
- In-process table run orchestration.
- Postgres persistence for runs/hands/actions/resources.
- Basic replay redaction policy enforcement.

## Required Before Production Launch

### 1) Auth Hardening (MUST)
- Replace static token maps with per-user auth (JWT or equivalent).
- Enforce ownership checks on agents, agent versions, tables, seats, and history routes.
- Implement token rotation, revocation, and expiration strategy.
- Add service-to-service auth for internal calls if split services are introduced.

### 2) Agent Sandboxing Hardening (MUST)
- Enforce network egress restrictions for agent execution environments.
- Apply strict CPU/memory/time quotas per action callback.
- Restrict syscalls and runtime capabilities (container/runtime hardening).
- Isolate agent workloads from control-plane and persistence network paths.

### 3) Secret Handling (MUST)
- Do not store user tokens in plaintext.
- Store credential material using hashing + pepper or KMS-backed secrets.
- Remove secrets from logs and error payloads.

### 4) Logging and Observability (MUST)
- Structured logs with request IDs, table IDs, run IDs, and hand IDs.
- Explicit logs for agent callback failures/timeouts/illegal actions.
- Persistence write/read failure logging with actionable context.
- Metrics/traces for callback latency, fallback rates, and run success/failure.

### 5) Reliability Architecture (MUST)
- Move long-running run execution to background workers/queue model.
- Add idempotency keys for mutating API requests.
- Add retry policies with bounded backoff and dead-letter handling.
- Ensure crash-safe run recovery semantics.

### 6) Data Integrity and Migration Safety (MUST)
- Use transactional write boundaries for related run/hand/action updates.
- Add migration tooling/version policy with rollback guidance.
- Define schema compatibility strategy for zero/low downtime upgrades.

### 7) Abuse Controls (MUST)
- Add rate limits per token and per endpoint class.
- Add payload size and endpoint validation hardening across all routes.
- Add allow/deny controls for agent callback domains and protocols.

### 8) Backup and Disaster Recovery (MUST)
- Define backup cadence and retention for Postgres.
- Verify restore procedures regularly.
- Define RPO/RTO and operational runbook.

### 9) Security and Compliance Validation (MUST)
- SAST/DAST and dependency vulnerability scanning in CI.
- Infrastructure and secrets scanning on deployment pipeline.
- Pre-launch external security review or penetration test.

## Fast Path to Production
1. Implement per-user auth + ownership checks first.
2. Add structured logging and metrics next.
3. Move runner execution to worker queue model.
4. Add sandboxing controls and abuse limits.
5. Finalize incident response + backup/restore drills before launch.
