# Poker Arena Prototype Spec (v0)

## 1. Goal
Build a functional prototype where AI agents can:
- be registered by a user,
- join a single poker table,
- receive per-turn game state,
- submit actions,
- complete hands end-to-end.

This version is intentionally minimal and optimized for speed of implementation.

## 2. Scope
### In scope
- One game type: No-Limit Texas Hold'em.
- One table at a time (up to 6 seats).
- Cash-game style continuous hands.
- HTTP JSON API for user and table operations.
- HTTP JSON callback protocol from engine to agent endpoint.
- Persistent storage for agents, tables, hands, and actions.
- Product mode: admin-first control plane.
- Product mode: signed-in human observer can watch their own agent's play history and table state.

### Out of scope (for now)
- Tournaments.
- Advanced auth and security hardening.
- Sandbox isolation for untrusted agent code.
- Anti-collusion and fraud systems.
- Leaderboards and ratings.
- Live human turn action submission to the engine.

## 3. Fixed Rules for v0
- Seats: max 6 players.
- Minimum players to start hand: 2.
- Starting stack per seated agent: 10,000 chips.
- Blinds: small blind 50, big blind 100.
- Blind schedule: fixed.
- Action timeout: 2 seconds.
- Timeout fallback: `check` if legal; otherwise `fold`.
- Invalid action fallback: same as timeout fallback.

## 4. Agent Protocol (v1)
Engine sends POST to agent endpoint:
- Path: agent-defined URL.
- Timeout: 2 seconds.
- Content-Type: `application/json`.

Request payload:
- `protocol_version` (number, always `1`)
- `hand_id` (string)
- `table_id` (string)
- `seat` (number)
- `hole_cards` (array of 2 strings)
- `board` (array of card strings)
- `pot` (number)
- `to_call` (number)
- `min_raise_to` (number or null)
- `stacks` (map seat -> chips)
- `bets` (map seat -> chips in current round)
- `legal_actions` (array of `fold|check|call|bet|raise`)
- `action_deadline_ms` (number)

Response payload:
- `action` (`fold|check|call|bet|raise`)
- `amount` (number, required for `bet`/`raise`, otherwise omitted)

On timeout, network error, malformed payload, or illegal action:
- Engine applies fallback action.

## 5. Minimal API Surface
- `POST /users`
- `POST /agents`
- `POST /agents/:id/versions`
- `POST /tables`
- `POST /tables/:id/join`
- `GET /tables/:id/state`
- `POST /tables/:id/start` (starts loop for this table)
- `POST /tables/:id/stop` (stops loop for this table)
- `GET /tables/:id/hands` (observer-visible hand history)
- `GET /hands/:id/actions` (observer-visible action history)
- `GET /hands/:id/replay` (observer-visible replay with visibility controls)

## 6. Data Model (initial)
- `users(id, name, token, created_at)`
- `agents(id, user_id, name, created_at)`
- `agent_versions(id, agent_id, version, endpoint_url, config_json, created_at)`
- `tables(id, name, max_seats, small_blind, big_blind, status, created_at)`
- `seats(id, table_id, seat_no, agent_id, agent_version_id, stack, status)`
- `hands(id, table_id, hand_no, button_seat, state_json, winner_summary_json, created_at, ended_at)`
- `actions(id, hand_id, street, acting_seat, action, amount, is_fallback, created_at)`

## 7. Done Criteria
Prototype is considered complete when:
1. At least 2 agents can register and join the same table.
2. Engine starts and completes 100 consecutive hands without crashing.
3. Every hand persists action history and final stack updates.
4. Chip conservation holds across all completed hands.

## 8. Implementation Notes for Next Phase
Keep these decisions now so hardening is easier later:
- Authoritative server-side game engine.
- Immutable `agent_versions`; hand binds to exact version id.
- Append-only hand/action history.
- Versioned agent protocol (`protocol_version`).
