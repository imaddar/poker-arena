# Code Audit Report — 2026-02-16

Scope: `services/engine` (API server, runner, rules/state machine, and persistence interactions).

## Findings

### 1) Final run status persistence failures are silently ignored (high)
- In `runTable`, the final `UpsertTableRun` error is discarded (`_ = s.repo.UpsertTableRun(finalStatus)`), so a failed write can leave API consumers with stale state and no signal that persistence failed.
- Similar silent writes exist in failure paths (`failBeforeRun`, `failRun`).
- Impact: operationally confusing states (`running` vs `completed/failed`) and hard-to-debug production incidents.

### 2) `GET /hands/{hand_id}/actions` cannot distinguish missing hand from empty action list (medium)
- `handleActions` directly proxies `repo.ListActions(handID)` and always returns `200` with an array.
- The in-memory repo returns `nil`/empty for unknown `handID` instead of `ErrHandNotFound`.
- Impact: clients cannot tell "hand exists with no actions yet" from "hand does not exist", which can break polling and error handling semantics.

### 3) Status endpoint does an O(hands) + per-hand query scan on every request (medium)
- `handleStatus` loads all hands and then calls `ListActions` for each hand to compute `ActionsPersisted`.
- Impact: status checks get progressively slower as hand history grows (N+1 query pattern). This can become a bottleneck for long runs.

### 4) Table config validation allows zero blind structures that later fail at runtime (medium)
- `TableConfig.Validate` only checks `BigBlind >= SmallBlind` and misses lower bounds (`> 0`).
- `StartNewHand` then fails only later when both posted blinds are zero (`failed to post blinds`).
- Impact: invalid game configuration passes initial validation and fails deeper in execution, producing harder-to-understand failures.

### 5) `starting_hand` accepts zero without validation (low/medium)
- API validation sets default `starting_hand=1` but does not reject explicitly provided `0`.
- Impact: creates hand sequence starting at `0`, which is inconsistent with default semantics and may break downstream assumptions/reporting.

### 6) Agent host allowlist compares full `host:port` and can reject expected hosts (low)
- Validation checks `parsedEndpoint.Host` directly against `AllowedAgentHosts`; this value includes port.
- If operators configure allowlist as hostnames only (common), endpoints are rejected unless each `host:port` variant is explicitly added.
- Impact: brittle deployment config and avoidable startup failures.

## Recommended next steps
1. Make repository writes in run-finalization and failure paths strict (return/log and surface persistence errors).
2. Introduce `GetHand`/existence check and return 404 for unknown hand IDs in `/hands/{id}/actions`.
3. Persist action counts incrementally (or store table aggregates) to remove status N+1 scans.
4. Extend `TableConfig.Validate` to require `small_blind > 0`, `big_blind > 0`, and likely `big_blind >= small_blind`.
5. Reject `starting_hand == 0` in request validation.
6. Normalize allowlist comparison to hostname (and optionally explicit port policy).
