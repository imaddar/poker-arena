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
