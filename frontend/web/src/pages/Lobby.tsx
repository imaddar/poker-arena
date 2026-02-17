import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../api';
import { filterTables, type LobbyFilters, type StakeFilter, type StatusFilter } from '../lib/pokerLogic';
import { formatArchiveTableId } from '../lib/presentation';
import type { Table } from '../types';

const DEFAULT_FILTERS: LobbyFilters = {
  status: 'all',
  stake: 'all',
  query: '',
};

export function Lobby() {
  const [tables, setTables] = useState<Table[]>([]);
  const [filters, setFilters] = useState<LobbyFilters>(DEFAULT_FILTERS);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();

  const loadTables = async () => {
    setIsLoading(true);
    setError(null);

    try {
      const next = await api.getTables();
      setTables(next);
    } catch (caught) {
      console.error(caught);
      setError('Unable to load archive directory.');
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    void loadTables();
  }, []);

  const visibleTables = useMemo(() => filterTables({ tables, filters }), [tables, filters]);

  const handleJoin = async (tableId: string) => {
    const result = await api.joinTable(tableId);
    if (!result.success) {
      setError(result.message ?? 'Unable to initialize this room right now.');
      return;
    }

    navigate(`/game/${tableId}`);
  };

  const setStatus = (status: StatusFilter) => setFilters((prev) => ({ ...prev, status }));
  const setStake = (stake: StakeFilter) => setFilters((prev) => ({ ...prev, stake }));

  return (
    <section>
      <h2>Open Table Archive</h2>
      <span className="sub-header">FILTER BY VOLATILITY_INDEX // SELECT PROTOCOL</span>

      <div className="lobby-controls">
        <input
          type="text"
          value={filters.query}
          placeholder="SEARCH_ROOM_NAME"
          onChange={(event) => setFilters((prev) => ({ ...prev, query: event.target.value }))}
        />

        <select value={filters.status} onChange={(event) => setStatus(event.target.value as StatusFilter)}>
          <option value="all">STATUS: ALL</option>
          <option value="waiting">STATUS: WAITING</option>
          <option value="playing">STATUS: PLAYING</option>
        </select>

        <select value={filters.stake} onChange={(event) => setStake(event.target.value as StakeFilter)}>
          <option value="all">STAKES: ALL</option>
          <option value="low">STAKES: LOW</option>
          <option value="mid">STAKES: MID</option>
          <option value="high">STAKES: HIGH</option>
        </select>

        <button type="button" className="enter-btn" onClick={loadTables} disabled={isLoading}>
          {isLoading ? 'SYNCING...' : 'REFRESH'}
        </button>
      </div>

      {error && <p className="error-text">{error}</p>}

      <div className="table-list">
        {visibleTables.map((table) => {
          const full = table.players >= table.maxSeats;

          return (
            <div className="table-row" key={table.id}>
              <span className="id-label">{formatArchiveTableId(table.id)}</span>
              <span className="room-name">{table.name.toUpperCase()}</span>
              <span className="table-meta">
                LOAD: {table.players}/{table.maxSeats} AGENTS // BLINDS: {table.smallBlind}/{table.bigBlind}
              </span>
              <button
                type="button"
                className="enter-btn"
                onClick={() => handleJoin(table.id)}
                disabled={full}
                style={full ? { opacity: 0.3, cursor: 'not-allowed' } : undefined}
              >
                {full ? 'FULL_CAPACITY' : 'INITIALIZE'}
              </button>
            </div>
          );
        })}

        {!isLoading && visibleTables.length === 0 && <p className="empty-text">NO_TABLES_MATCH_FILTERS</p>}
      </div>
    </section>
  );
}
