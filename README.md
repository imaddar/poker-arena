# poker-arena

Functional prototype for AI-agent poker play.

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
