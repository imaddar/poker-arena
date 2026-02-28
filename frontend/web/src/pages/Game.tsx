import { useEffect, useMemo, useState } from 'react';
import { ArrowLeft, RefreshCw } from 'lucide-react';
import { useNavigate, useParams } from 'react-router-dom';
import { api } from '../api';
import { resolveApiRuntimeConfig } from '../api/config';
import { Card } from '../components/Card';
import { PokerTable } from '../components/PokerTable';
import { useAuth } from '../contexts/AuthContext';
import { clampRaiseAmount } from '../lib/pokerLogic';
import { formatArchiveTableId } from '../lib/presentation';
import { loadGameState } from './gameLoader';
import type { HandReplay, LatestReplay, PendingAdminAction } from '../api/types';
import type { ActionType, GameState } from '../types';

export function Game() {
  const { tableId } = useParams<{ tableId: string }>();
  const navigate = useNavigate();
  const { user } = useAuth();
  const isMockMode = resolveApiRuntimeConfig(import.meta.env).useMock;
  const isAdmin = user?.role === 'admin';

  const [state, setState] = useState<GameState | null>(null);
  const [raiseAmount, setRaiseAmount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [logOpen, setLogOpen] = useState(false);
  const [latestReplay, setLatestReplay] = useState<LatestReplay | null>(null);
  const [adminReplay, setAdminReplay] = useState<HandReplay | null>(null);
  const [liveCursor, setLiveCursor] = useState(0);
  const [adminSeatNo, setAdminSeatNo] = useState(1);
  const [pendingAdminAction, setPendingAdminAction] = useState<PendingAdminAction | null>(null);

  const loadAdminReplay = async (latest: LatestReplay | null) => {
    if (!latest?.handId || !isAdmin || isMockMode) {
      setAdminReplay(null);
      return;
    }

    const replay = await api.getHandReplay(latest.handId);
    setAdminReplay(replay);
  };

  const loadState = async () => {
    if (!tableId) {
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const loaded = await loadGameState(api, tableId, isMockMode);
      setState(loaded.state);
      setRaiseAmount(loaded.raiseAmount);
      const latest = loaded.latestReplay ?? null;
      setLatestReplay(latest);
      await loadAdminReplay(latest);
      if (!isMockMode) {
        const live = await api.getTableLive(tableId, 0);
        setLiveCursor(live.nextActionCursor);
        setState((prev) =>
          prev
            ? {
                ...prev,
                handId: live.handId ?? prev.handId,
                actionLog: live.actionLog.length > 0 ? live.actionLog : prev.actionLog,
              }
            : prev,
        );
      }
      if (!isMockMode && isAdmin) {
        const pending = await api.getPendingAdminAction(tableId, adminSeatNo);
        setPendingAdminAction(pending);
      }
    } catch (caught) {
      console.error(caught);
      setError('Could not load live schematic.');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadState();
  }, [tableId, isAdmin]);

  useEffect(() => {
    if (isMockMode || !tableId || !isAdmin) {
      return;
    }
    const interval = setInterval(() => {
      void (async () => {
        if (!tableId) {
          return;
        }
        try {
          const live = await api.getTableLive(tableId, liveCursor);
          setLiveCursor(live.nextActionCursor);
          if (live.handId && live.handId !== state?.handId) {
            await loadState();
            return;
          }
          if (live.actionLog.length > 0) {
            setState((prev) =>
              prev
                ? {
                    ...prev,
                    handId: live.handId ?? prev.handId,
                    actionLog: [...prev.actionLog, ...live.actionLog],
                  }
                : prev,
            );
          }
          await loadAdminReplay(latestReplay);
          const pending = await api.getPendingAdminAction(tableId, adminSeatNo);
          setPendingAdminAction(pending);
        } catch {
          // Keep prior state and retry on next poll tick.
        }
      })();
    }, 3000);
    return () => clearInterval(interval);
  }, [isMockMode, tableId, isAdmin, liveCursor, state?.handId, latestReplay, adminSeatNo]);

  const streetLabel = useMemo(() => state?.street.toUpperCase() ?? '-', [state]);

  const handleAction = async (type: ActionType, amount?: number) => {
    if (!tableId || !state) {
      return;
    }
    if (!isMockMode) {
      setError('Observer mode: actions are disabled for backend-backed tables.');
      return;
    }

    const next = await api.submitAction(tableId, { type, amount });
    setState(next);
    setRaiseAmount(
      clampRaiseAmount({
        requested: next.minRaise,
        minRaise: next.minRaise,
        stack: next.seats[next.currentTurnSeat]?.stack ?? 0,
      }),
    );
  };

  const handleLeave = async () => {
    if (tableId) {
      await api.leaveTable(tableId);
    }

    navigate('/lobby');
  };

  const handleJoinAdminSeat = async () => {
    if (!tableId) {
      return;
    }
    try {
      await api.joinAdminSeat(tableId, adminSeatNo);
      setError(null);
      const pending = await api.getPendingAdminAction(tableId, adminSeatNo);
      setPendingAdminAction(pending);
    } catch (caught) {
      console.error(caught);
      setError('Failed to join admin seat.');
    }
  };

  const handleSubmitAdminAction = async (action: string, amount?: number) => {
    if (!tableId || !pendingAdminAction) {
      return;
    }
    try {
      await api.submitAdminAction(tableId, pendingAdminAction.seatNo, action, amount);
      setError(null);
      await loadState();
    } catch (caught) {
      console.error(caught);
      setError('Failed to submit admin action.');
    }
  };

  if (loading && !state) {
    return (
      <div className="loading-state">
        <RefreshCw size={18} className="spin" />
        <span>SYNCING_LIVE_SCHEMATIC...</span>
      </div>
    );
  }

  if (!state) {
    return <p className="error-text">{error ?? 'Table unavailable.'}</p>;
  }

  const tableTitle = state.tableName ?? formatArchiveTableId(state.tableId);

  return (
    <section className="game-screen">
      <header className="table-head">
        <button type="button" className="enter-btn" onClick={handleLeave}>
          <ArrowLeft size={14} /> LEAVE_TABLE
        </button>
        <div className="table-head__title">{tableTitle}</div>
        <button type="button" className="enter-btn" onClick={loadState}>
          <RefreshCw size={14} /> REFRESH_STATE
        </button>
      </header>

      {error && <p className="error-text">{error}</p>}
      {!isMockMode && <p className="sub-header">OBSERVER_MODE // ACTIONS_DISABLED</p>}
      {!isMockMode && (
        <p className="sub-header">
          LATEST_HAND: #{latestReplay?.handNo ?? '-'} // ACTIONS: {latestReplay?.totalActions ?? 0} // FALLBACKS:{' '}
          {latestReplay?.fallbackActions ?? 0}
        </p>
      )}
      {!isMockMode && isAdmin && adminReplay && (
        <section className="ledger-box" aria-label="Admin replay inspector">
          <h3>Admin Replay Inspector</h3>
          <p className="ledger-note">HAND {adminReplay.handId} // FULL_CARDS_VISIBLE // AUTO_REFRESH_3S</p>
          <div className="community-board">
            <strong>BOARD:</strong>{' '}
            {adminReplay.board.length > 0 ? (
              adminReplay.board.map((card, index) => <Card key={`board-${index}`} card={card} compact />)
            ) : (
              <span>NO_BOARD_CARDS</span>
            )}
          </div>
          <div>
            <strong>SEAT_CARDS:</strong>
            {Object.keys(adminReplay.holeCardsBySeat)
              .map((seat) => Number(seat))
              .sort((a, b) => a - b)
              .map((seatNo) => (
                <div key={`seat-cards-${seatNo}`} style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                  <span>S{seatNo}:</span>
                  {(adminReplay.holeCardsBySeat[seatNo] ?? []).length > 0 ? (
                    adminReplay.holeCardsBySeat[seatNo].map((card, idx) => (
                      <Card key={`seat-${seatNo}-card-${idx}`} card={card} compact />
                    ))
                  ) : (
                    <span>NO_CARDS</span>
                  )}
                </div>
              ))}
          </div>
          <div>
            <strong>DECISIONS:</strong>
            <ol>
              {adminReplay.actions.map((action, idx) => {
                const amount = action.amount == null ? '' : ` ${action.amount}`;
                const fallback = action.isFallback ? ' (fallback)' : '';
                return (
                  <li key={`decision-${idx}`}>
                    {action.street.toUpperCase()} S{action.actingSeat}: {action.action.toUpperCase()}
                    {amount}
                    {fallback}
                  </li>
                );
              })}
            </ol>
          </div>
        </section>
      )}
      {!isMockMode && isAdmin && (
        <section className="ledger-box" aria-label="Admin action controls">
          <h3>Admin Seat Controls</h3>
          <div className="lobby-controls">
            <label htmlFor="admin-seat-no">SEAT_NO</label>
            <input
              id="admin-seat-no"
              type="number"
              min={1}
              max={6}
              value={adminSeatNo}
              onChange={(event) => setAdminSeatNo(Math.max(1, Number(event.target.value) || 1))}
            />
            <button type="button" className="enter-btn" onClick={handleJoinAdminSeat}>
              JOIN_ADMIN_SEAT
            </button>
          </div>
          {pendingAdminAction ? (
            <div>
              <p className="ledger-note">
                PENDING_ACTION // HAND={pendingAdminAction.handId} // SEAT={pendingAdminAction.seatNo} // TO_CALL=
                {pendingAdminAction.toCall} // POT={pendingAdminAction.pot}
              </p>
              <div className="action-stack">
                {pendingAdminAction.legalActions.map((action) => (
                  <button key={`admin-action-${action}`} type="button" className="enter-btn" onClick={() => handleSubmitAdminAction(action)}>
                    {action.toUpperCase()}
                  </button>
                ))}
                {pendingAdminAction.legalActions.includes('raise') && pendingAdminAction.minRaiseTo != null && (
                  <button
                    type="button"
                    className="enter-btn"
                    onClick={() => handleSubmitAdminAction('raise', pendingAdminAction.minRaiseTo)}
                  >
                    RAISE_TO_{pendingAdminAction.minRaiseTo}
                  </button>
                )}
              </div>
            </div>
          ) : (
            <p className="ledger-note">No pending admin action for this seat yet.</p>
          )}
        </section>
      )}

      <div className="game-grid">
        <PokerTable
          gameState={state}
          tableInfo={`ROOM: ${formatArchiveTableId(state.tableId)} // HAND: ${state.handId} // STREET: ${streetLabel}`}
          raiseAmount={raiseAmount}
          onRaiseAmountChange={setRaiseAmount}
          onAction={handleAction}
        />
      </div>

      <aside className={`log-drawer ${logOpen ? 'open' : ''}`} aria-label="Hand log">
        <button type="button" className="log-drawer-toggle" onClick={() => setLogOpen((prev) => !prev)}>
          {logOpen ? 'HIDE_LOG' : 'SHOW_LOG'}
        </button>
        <div className="log-panel">
          <h3>ACTION_LOG</h3>
          <ol>
            {state.actionLog.map((entry, index) => (
              <li key={`${entry}-${index}`}>{entry}</li>
            ))}
          </ol>
        </div>
      </aside>
    </section>
  );
}
