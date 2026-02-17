import { useEffect, useMemo, useState } from 'react';
import { ArrowLeft, RefreshCw } from 'lucide-react';
import { useNavigate, useParams } from 'react-router-dom';
import { api } from '../api';
import { PokerTable } from '../components/PokerTable';
import { clampRaiseAmount } from '../lib/pokerLogic';
import { formatArchiveTableId } from '../lib/presentation';
import type { ActionType, GameState } from '../types';

export function Game() {
  const { tableId } = useParams<{ tableId: string }>();
  const navigate = useNavigate();

  const [state, setState] = useState<GameState | null>(null);
  const [raiseAmount, setRaiseAmount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [logOpen, setLogOpen] = useState(false);

  const loadState = async () => {
    if (!tableId) {
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const nextState = await api.getTableState(tableId);
      setState(nextState);
      setRaiseAmount(
        clampRaiseAmount({
          requested: nextState.minRaise,
          minRaise: nextState.minRaise,
          stack: nextState.seats[2]?.stack ?? 0,
        }),
      );
    } catch (caught) {
      console.error(caught);
      setError('Could not load live schematic.');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadState();
  }, [tableId]);

  const streetLabel = useMemo(() => state?.street.toUpperCase() ?? '-', [state]);

  const handleAction = async (type: ActionType, amount?: number) => {
    if (!tableId || !state) {
      return;
    }

    const next = await api.submitAction(tableId, { type, amount });
    setState(next);
    setRaiseAmount(
      clampRaiseAmount({
        requested: next.minRaise,
        minRaise: next.minRaise,
        stack: next.seats[2]?.stack ?? 0,
      }),
    );
  };

  const handleLeave = async () => {
    if (tableId) {
      await api.leaveTable(tableId);
    }

    navigate('/lobby');
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
