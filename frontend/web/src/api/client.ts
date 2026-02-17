import type { ActionRequest, GameState, Player, Table, User } from '../types';
import type { ApiClient } from './types';

interface HttpApiClientOptions {
  baseUrl: string;
  getToken: () => string;
}

interface TableDTO {
  id: string;
  name: string;
  max_seats: number;
  small_blind: number;
  big_blind: number;
  status: string;
}

interface SeatDTO {
  seat_no: number;
  stack: number;
  status: string;
}

interface TableStateDTO {
  table: TableDTO;
  seats: SeatDTO[];
}

function normalizeBaseUrl(raw: string): string {
  const trimmed = raw.trim();
  if (!trimmed) {
    return '';
  }
  return trimmed.endsWith('/') ? trimmed.slice(0, -1) : trimmed;
}

function mapTable(dto: TableDTO): Table {
  const status = dto.status === 'running' ? 'playing' : 'waiting';
  return {
    id: dto.id,
    name: dto.name,
    maxSeats: dto.max_seats,
    smallBlind: dto.small_blind,
    bigBlind: dto.big_blind,
    status,
    players: 0,
  };
}

function mapSeatToPlayer(seat: SeatDTO): Player {
  const status: Player['status'] = seat.status === 'active' ? 'active' : 'sitting_out';
  return {
    id: `seat-${seat.seat_no}`,
    name: `Seat ${seat.seat_no}`,
    seat: seat.seat_no - 1,
    stack: seat.stack,
    bet: 0,
    status,
    isDealer: false,
    isTurn: false,
  };
}

function mapTableState(dto: TableStateDTO): GameState {
  const maxSeats = Math.max(6, dto.table.max_seats);
  const seats: (Player | null)[] = Array.from({ length: maxSeats }, () => null);
  for (const seat of dto.seats) {
    const index = seat.seat_no - 1;
    if (index < 0 || index >= seats.length) {
      continue;
    }
    seats[index] = mapSeatToPlayer(seat);
  }

  return {
    tableId: dto.table.id,
    tableName: dto.table.name,
    handId: 'table-state',
    street: 'preflop',
    communityCards: [],
    pot: 0,
    currentTurnSeat: -1,
    dealerSeat: 0,
    seats,
    toCall: 0,
    minRaise: dto.table.big_blind,
    actionLog: [`Loaded table state for ${dto.table.name}`],
  };
}

export function createHttpApiClient(options: HttpApiClientOptions): ApiClient {
  const baseUrl = normalizeBaseUrl(options.baseUrl);

  const request = async <T>(path: string, init?: RequestInit): Promise<T> => {
    if (!baseUrl) {
      throw new Error('VITE_API_BASE_URL is required when VITE_USE_MOCK_API is false.');
    }

    const headers = new Headers(init?.headers);
    headers.set('Content-Type', 'application/json');

    const token = options.getToken();
    if (token) {
      headers.set('Authorization', `Bearer ${token}`);
    }

    const response = await fetch(`${baseUrl}${path}`, {
      ...init,
      headers,
    });

    if (!response.ok) {
      const body = await response.text();
      throw new Error(`API request failed (${response.status}): ${body}`);
    }

    return (await response.json()) as T;
  };

  return {
    async login(username: string): Promise<User> {
      const token = options.getToken();
      if (!token) {
        throw new Error('Missing VITE_ADMIN_TOKEN for control-plane authentication.');
      }
      return {
        id: `local-${username.trim().toLowerCase().replace(/\s+/g, '-')}`,
        name: username.trim(),
        token,
      };
    },

    async getTables(): Promise<Table[]> {
      const rows = await request<TableDTO[]>('/tables', { method: 'GET' });
      return rows.map(mapTable);
    },

    async joinTable(_tableId: string): Promise<{ success: boolean; message?: string }> {
      return { success: true };
    },

    async leaveTable(_tableId: string): Promise<{ success: boolean }> {
      return { success: true };
    },

    async getTableState(tableId: string): Promise<GameState> {
      const state = await request<TableStateDTO>(`/tables/${tableId}/state`, { method: 'GET' });
      return mapTableState(state);
    },

    async submitAction(tableId: string, _action: ActionRequest): Promise<GameState> {
      return this.getTableState(tableId);
    },
  };
}
