CREATE TABLE IF NOT EXISTS table_runs (
  table_id TEXT PRIMARY KEY,
  status TEXT NOT NULL CHECK (status IN ('idle','running','stopped','failed','completed')),
  started_at TIMESTAMPTZ NOT NULL,
  ended_at TIMESTAMPTZ NULL,
  error TEXT NOT NULL DEFAULT '',
  hands_requested INTEGER NOT NULL,
  hands_completed INTEGER NOT NULL,
  total_actions INTEGER NOT NULL,
  total_fallbacks INTEGER NOT NULL,
  current_hand_no BIGINT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_table_runs_status ON table_runs(status);
CREATE INDEX IF NOT EXISTS idx_table_runs_updated_at ON table_runs(updated_at DESC);

CREATE TABLE IF NOT EXISTS hands (
  hand_id TEXT PRIMARY KEY,
  table_id TEXT NOT NULL REFERENCES table_runs(table_id) ON DELETE CASCADE,
  hand_no BIGINT NOT NULL,
  started_at TIMESTAMPTZ NOT NULL,
  ended_at TIMESTAMPTZ NULL,
  final_phase TEXT NOT NULL,
  final_state JSONB NOT NULL,
  winner_summary JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (table_id, hand_no)
);

CREATE INDEX IF NOT EXISTS idx_hands_table_handno ON hands(table_id, hand_no ASC);
CREATE INDEX IF NOT EXISTS idx_hands_table_started ON hands(table_id, started_at ASC);

CREATE TABLE IF NOT EXISTS actions (
  id BIGSERIAL PRIMARY KEY,
  hand_id TEXT NOT NULL REFERENCES hands(hand_id) ON DELETE CASCADE,
  street TEXT NOT NULL,
  acting_seat SMALLINT NOT NULL,
  action TEXT NOT NULL,
  amount INTEGER NULL,
  is_fallback BOOLEAN NOT NULL DEFAULT false,
  at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_actions_hand_id_id ON actions(hand_id, id ASC);
CREATE INDEX IF NOT EXISTS idx_actions_hand_at ON actions(hand_id, at ASC);
