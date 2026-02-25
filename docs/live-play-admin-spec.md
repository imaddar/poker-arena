# Live Play + Admin Participation Spec (v0)

## Goal

Enable three workflows locally:

1. Run local agents against each other continuously.
2. Watch hands live in the frontend as actions happen.
3. Let admin join a table as a playable seat (admin-only) and act against agents.

## Non-Goals (this phase)

- Public multi-user matchmaking.
- Real-money or security hardening changes.
- WebSocket-only architecture (polling is acceptable first).

## Roles

- `admin`
  - full control-plane operations
  - can watch all table details
  - can join playable human-admin seat
- `seat observer`
  - can view history/replay/live hand data only for participating seat
  - cannot execute control-plane actions

## Phase 1: Live Viewing Contract

Add `GET /tables/:id/live` for poll-based frontend updates.

Query params:

- `after_action` (optional, integer >= 0): return only actions with index >= cursor.

Response shape:

- `table` (table metadata)
- `latest_hand` (optional)
- `actions` (action list for latest visible hand from `after_action`)
- `next_action_cursor` (integer cursor for next poll)

Behavior:

- If no hands exist, return empty `actions` and cursor `0`.
- For admin: use latest table hand.
- For seat token: use latest hand that includes that seat.
- Reject invalid cursor values (`< 0`, non-integer, or beyond available actions).

## Phase 2: Admin Playable Seat

Add admin-only human seat support:

- join table as `human_admin` seat type
- fetch pending action state for that seat
- submit legal action (`fold/check/call/bet/raise`) through backend route
- engine applies normal validation + timeout fallback when needed

## Phase 3: Frontend Live Table Viewer

Admin game page should show:

- live board cards
- seat cards (for admin replay visibility)
- per-action decision feed
- auto-refresh poll loop using `GET /tables/:id/live`

Observer game page should remain view-only.

## Acceptance Tests

### A. Live endpoint contract

1. `GET /tables/:id/live` with no history returns:
   - `200`
   - `actions=[]`
   - `next_action_cursor=0`
2. With persisted actions and `after_action=1`:
   - `200`
   - returns actions starting from index 1
   - `next_action_cursor` equals total actions for the hand
3. Invalid `after_action`:
   - `400`

### B. Role boundaries for live view

1. Admin can view latest table hand live.
2. Seat token sees latest participating hand only.
3. Seat token cannot access admin control-plane endpoints.

### C. End-to-end (post Phase 2 + 3)

1. Two local agents play, frontend updates action stream while run is active.
2. Admin joins one seat and submits at least one legal action from UI.
3. Hand history/replay remains queryable after completion.
