package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/imaddar/poker-arena/services/engine/internal/domain"
)

type postgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) UpsertTableRun(record TableRunRecord) error {
	const q = `
INSERT INTO table_runs (
  table_id, status, started_at, ended_at, error, hands_requested, hands_completed, total_actions, total_fallbacks, current_hand_no, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now())
ON CONFLICT (table_id) DO UPDATE SET
  status = EXCLUDED.status,
  started_at = EXCLUDED.started_at,
  ended_at = EXCLUDED.ended_at,
  error = EXCLUDED.error,
  hands_requested = EXCLUDED.hands_requested,
  hands_completed = EXCLUDED.hands_completed,
  total_actions = EXCLUDED.total_actions,
  total_fallbacks = EXCLUDED.total_fallbacks,
  current_hand_no = EXCLUDED.current_hand_no,
  updated_at = now()
`
	_, err := r.db.ExecContext(context.Background(), q,
		record.TableID,
		string(record.Status),
		record.StartedAt,
		record.EndedAt,
		record.Error,
		record.HandsRequested,
		record.HandsCompleted,
		record.TotalActions,
		record.TotalFallbacks,
		record.CurrentHandNo,
	)
	return err
}

func (r *postgresRepository) GetTableRun(tableID string) (TableRunRecord, bool, error) {
	const q = `
SELECT table_id, status, started_at, ended_at, error, hands_requested, hands_completed, total_actions, total_fallbacks, current_hand_no
FROM table_runs
WHERE table_id = $1
`
	var status string
	var out TableRunRecord
	err := r.db.QueryRowContext(context.Background(), q, tableID).Scan(
		&out.TableID,
		&status,
		&out.StartedAt,
		&out.EndedAt,
		&out.Error,
		&out.HandsRequested,
		&out.HandsCompleted,
		&out.TotalActions,
		&out.TotalFallbacks,
		&out.CurrentHandNo,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return TableRunRecord{}, false, nil
	}
	if err != nil {
		return TableRunRecord{}, false, err
	}
	out.Status = TableRunStatus(status)
	return out, true, nil
}

func (r *postgresRepository) CreateHand(record HandRecord) error {
	finalState, err := json.Marshal(record.FinalState)
	if err != nil {
		return fmt.Errorf("marshal final state: %w", err)
	}
	winnerSummary, err := json.Marshal(record.WinnerSummary)
	if err != nil {
		return fmt.Errorf("marshal winner summary: %w", err)
	}
	const q = `
INSERT INTO hands (
  hand_id, table_id, hand_no, started_at, ended_at, final_phase, final_state, winner_summary
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
`
	_, err = r.db.ExecContext(context.Background(), q,
		record.HandID,
		record.TableID,
		record.HandNo,
		record.StartedAt,
		record.EndedAt,
		string(record.FinalPhase),
		finalState,
		winnerSummary,
	)
	if isUniqueViolation(err) {
		return ErrHandAlreadyExists
	}
	return err
}

func (r *postgresRepository) GetHand(handID string) (HandRecord, bool, error) {
	const q = `
SELECT hand_id, table_id, hand_no, started_at, ended_at, final_phase, final_state, winner_summary
FROM hands
WHERE hand_id = $1
`
	var rec HandRecord
	var finalPhase string
	var finalStateRaw []byte
	var winnerSummaryRaw []byte
	err := r.db.QueryRowContext(context.Background(), q, handID).Scan(
		&rec.HandID,
		&rec.TableID,
		&rec.HandNo,
		&rec.StartedAt,
		&rec.EndedAt,
		&finalPhase,
		&finalStateRaw,
		&winnerSummaryRaw,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return HandRecord{}, false, nil
	}
	if err != nil {
		return HandRecord{}, false, err
	}
	rec.FinalPhase = domain.HandPhase(finalPhase)
	if len(finalStateRaw) > 0 {
		if err := json.Unmarshal(finalStateRaw, &rec.FinalState); err != nil {
			return HandRecord{}, false, fmt.Errorf("unmarshal final_state for hand %s: %w", rec.HandID, err)
		}
	}
	if len(winnerSummaryRaw) > 0 {
		if err := json.Unmarshal(winnerSummaryRaw, &rec.WinnerSummary); err != nil {
			return HandRecord{}, false, fmt.Errorf("unmarshal winner_summary for hand %s: %w", rec.HandID, err)
		}
	}
	return rec, true, nil
}

func (r *postgresRepository) CompleteHand(handID string, final HandRecord) error {
	finalState, err := json.Marshal(final.FinalState)
	if err != nil {
		return fmt.Errorf("marshal final state: %w", err)
	}
	winnerSummary, err := json.Marshal(final.WinnerSummary)
	if err != nil {
		return fmt.Errorf("marshal winner summary: %w", err)
	}
	const q = `
UPDATE hands
SET table_id=$2, hand_no=$3, started_at=$4, ended_at=$5, final_phase=$6, final_state=$7, winner_summary=$8
WHERE hand_id = $1
`
	result, err := r.db.ExecContext(context.Background(), q,
		handID,
		final.TableID,
		final.HandNo,
		final.StartedAt,
		final.EndedAt,
		string(final.FinalPhase),
		finalState,
		winnerSummary,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrHandNotFound
	}
	return nil
}

func (r *postgresRepository) AppendAction(record ActionRecord) error {
	const q = `
INSERT INTO actions (
  hand_id, street, acting_seat, action, amount, is_fallback, at
) VALUES ($1,$2,$3,$4,$5,$6,$7)
`
	_, err := r.db.ExecContext(context.Background(), q,
		record.HandID,
		string(record.Street),
		int16(record.ActingSeat),
		string(record.Action),
		record.Amount,
		record.IsFallback,
		record.At,
	)
	if isForeignKeyViolation(err) {
		return ErrHandNotFound
	}
	return err
}

func (r *postgresRepository) ListHands(tableID string) ([]HandRecord, error) {
	const q = `
SELECT hand_id, table_id, hand_no, started_at, ended_at, final_phase, final_state, winner_summary
FROM hands
WHERE table_id = $1
ORDER BY hand_no ASC, hand_id ASC
`
	rows, err := r.db.QueryContext(context.Background(), q, tableID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]HandRecord, 0, 32)
	for rows.Next() {
		var rec HandRecord
		var finalPhase string
		var finalStateRaw []byte
		var winnerSummaryRaw []byte
		if err := rows.Scan(
			&rec.HandID,
			&rec.TableID,
			&rec.HandNo,
			&rec.StartedAt,
			&rec.EndedAt,
			&finalPhase,
			&finalStateRaw,
			&winnerSummaryRaw,
		); err != nil {
			return nil, err
		}
		rec.FinalPhase = domain.HandPhase(finalPhase)
		if len(finalStateRaw) > 0 {
			if err := json.Unmarshal(finalStateRaw, &rec.FinalState); err != nil {
				return nil, fmt.Errorf("unmarshal final_state for hand %s: %w", rec.HandID, err)
			}
		}
		if len(winnerSummaryRaw) > 0 {
			if err := json.Unmarshal(winnerSummaryRaw, &rec.WinnerSummary); err != nil {
				return nil, fmt.Errorf("unmarshal winner_summary for hand %s: %w", rec.HandID, err)
			}
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *postgresRepository) ListActions(handID string) ([]ActionRecord, error) {
	const q = `
SELECT hand_id, street, acting_seat, action, amount, is_fallback, at
FROM actions
WHERE hand_id = $1
ORDER BY id ASC
`
	rows, err := r.db.QueryContext(context.Background(), q, handID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ActionRecord, 0, 64)
	for rows.Next() {
		var rec ActionRecord
		var street string
		var action string
		var actingSeat int16
		var amount sql.NullInt32
		if err := rows.Scan(
			&rec.HandID,
			&street,
			&actingSeat,
			&action,
			&amount,
			&rec.IsFallback,
			&rec.At,
		); err != nil {
			return nil, err
		}
		rec.Street = domain.Street(street)
		rec.ActingSeat = domain.SeatNo(actingSeat)
		rec.Action = domain.ActionKind(action)
		if amount.Valid {
			value := uint32(amount.Int32)
			rec.Amount = &value
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *postgresRepository) CreateUser(record UserRecord) error {
	const q = `
INSERT INTO users (id, name, token, created_at)
VALUES ($1,$2,$3,$4)
`
	_, err := r.db.ExecContext(context.Background(), q,
		record.ID,
		record.Name,
		record.Token,
		record.CreatedAt,
	)
	return err
}

func (r *postgresRepository) CreateAgent(record AgentRecord) error {
	const q = `
INSERT INTO agents (id, user_id, name, created_at)
VALUES ($1,$2,$3,$4)
`
	_, err := r.db.ExecContext(context.Background(), q,
		record.ID,
		record.UserID,
		record.Name,
		record.CreatedAt,
	)
	if isForeignKeyViolation(err) {
		return ErrUserNotFound
	}
	return err
}

func (r *postgresRepository) CreateAgentVersion(record AgentVersionRecord) error {
	const q = `
INSERT INTO agent_versions (id, agent_id, version, endpoint_url, config_json, created_at)
VALUES ($1,$2,$3,$4,$5,$6)
`
	_, err := r.db.ExecContext(context.Background(), q,
		record.ID,
		record.AgentID,
		record.Version,
		record.EndpointURL,
		record.ConfigJSON,
		record.CreatedAt,
	)
	if isForeignKeyViolation(err) {
		return ErrAgentNotFound
	}
	if isUniqueViolation(err) {
		return ErrAgentVersionExists
	}
	return err
}

func (r *postgresRepository) CreateTable(record TableRecord) error {
	const q = `
INSERT INTO tables (id, name, max_seats, small_blind, big_blind, status, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
`
	_, err := r.db.ExecContext(context.Background(), q,
		record.ID,
		record.Name,
		int16(record.MaxSeats),
		int32(record.SmallBlind),
		int32(record.BigBlind),
		record.Status,
		record.CreatedAt,
	)
	return err
}

func (r *postgresRepository) UpsertSeat(record SeatRecord) error {
	version, ok, err := r.GetAgentVersion(record.AgentVersionID)
	if err != nil {
		return err
	}
	if !ok || version.AgentID != record.AgentID {
		return ErrAgentVersionNotFound
	}

	const q = `
INSERT INTO seats (id, table_id, seat_no, agent_id, agent_version_id, stack, status, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (table_id, seat_no) DO UPDATE SET
  id = EXCLUDED.id,
  agent_id = EXCLUDED.agent_id,
  agent_version_id = EXCLUDED.agent_version_id,
  stack = EXCLUDED.stack,
  status = EXCLUDED.status,
  created_at = EXCLUDED.created_at
`
	_, err = r.db.ExecContext(context.Background(), q,
		record.ID,
		record.TableID,
		int16(record.SeatNo),
		record.AgentID,
		record.AgentVersionID,
		int32(record.Stack),
		string(record.Status),
		record.CreatedAt,
	)
	if isForeignKeyViolation(err) {
		if strings.Contains(err.Error(), "table_id") {
			return ErrTableNotFound
		}
		if strings.Contains(err.Error(), "agent_id") {
			return ErrAgentNotFound
		}
		if strings.Contains(err.Error(), "agent_version_id") {
			return ErrAgentVersionNotFound
		}
	}
	return err
}

func (r *postgresRepository) GetTable(tableID string) (TableRecord, bool, error) {
	const q = `
SELECT id, name, max_seats, small_blind, big_blind, status, created_at
FROM tables
WHERE id = $1
`
	var rec TableRecord
	var maxSeats int16
	var smallBlind int32
	var bigBlind int32
	err := r.db.QueryRowContext(context.Background(), q, tableID).Scan(
		&rec.ID,
		&rec.Name,
		&maxSeats,
		&smallBlind,
		&bigBlind,
		&rec.Status,
		&rec.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return TableRecord{}, false, nil
	}
	if err != nil {
		return TableRecord{}, false, err
	}
	rec.MaxSeats = uint8(maxSeats)
	rec.SmallBlind = uint32(smallBlind)
	rec.BigBlind = uint32(bigBlind)
	return rec, true, nil
}

func (r *postgresRepository) ListSeats(tableID string) ([]SeatRecord, error) {
	const q = `
SELECT id, table_id, seat_no, agent_id, agent_version_id, stack, status, created_at
FROM seats
WHERE table_id = $1
ORDER BY seat_no ASC
`
	rows, err := r.db.QueryContext(context.Background(), q, tableID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]SeatRecord, 0, 8)
	for rows.Next() {
		var rec SeatRecord
		var seatNo int16
		var stack int32
		if err := rows.Scan(
			&rec.ID,
			&rec.TableID,
			&seatNo,
			&rec.AgentID,
			&rec.AgentVersionID,
			&stack,
			&rec.Status,
			&rec.CreatedAt,
		); err != nil {
			return nil, err
		}
		rec.SeatNo = domain.SeatNo(seatNo)
		rec.Stack = uint32(stack)
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *postgresRepository) GetAgentVersion(versionID string) (AgentVersionRecord, bool, error) {
	const q = `
SELECT id, agent_id, version, endpoint_url, config_json, created_at
FROM agent_versions
WHERE id = $1
`
	var rec AgentVersionRecord
	var config []byte
	err := r.db.QueryRowContext(context.Background(), q, versionID).Scan(
		&rec.ID,
		&rec.AgentID,
		&rec.Version,
		&rec.EndpointURL,
		&config,
		&rec.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentVersionRecord{}, false, nil
	}
	if err != nil {
		return AgentVersionRecord{}, false, err
	}
	rec.ConfigJSON = append([]byte(nil), config...)
	return rec, true, nil
}

func isUniqueViolation(err error) bool {
	return hasSQLState(err, "23505")
}

func isForeignKeyViolation(err error) bool {
	return hasSQLState(err, "23503")
}

type sqlStateProvider interface {
	SQLState() string
}

func hasSQLState(err error, code string) bool {
	if err == nil {
		return false
	}
	var stateErr sqlStateProvider
	if errors.As(err, &stateErr) && stateErr.SQLState() == code {
		return true
	}
	// Fallback for drivers that only surface SQLSTATE in error text.
	return strings.Contains(err.Error(), "SQLSTATE "+code)
}
