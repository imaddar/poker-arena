import type { ActionRequest, GameState, Table, User, UserRole } from '../types';

export interface HandSummary {
  handId: string;
  handNo: number;
  endedAt?: string;
}

export interface LatestReplay {
  handId?: string;
  handNo?: number;
  totalActions?: number;
  fallbackActions?: number;
  actionLog: string[];
}

export interface SessionInfo {
  role: UserRole;
  seatNo?: number;
}

export interface ReplayCard {
  rank: string;
  suit: 's' | 'h' | 'd' | 'c';
}

export interface ReplayAction {
  street: string;
  actingSeat: number;
  action: string;
  amount?: number;
  isFallback: boolean;
}

export interface HandReplay {
  handId: string;
  board: ReplayCard[];
  holeCardsBySeat: Record<number, ReplayCard[]>;
  actions: ReplayAction[];
}

export interface TableLive {
  handId?: string;
  handNo?: number;
  actionLog: string[];
  nextActionCursor: number;
}

export interface ApiClient {
  login(username: string, authToken?: string): Promise<User>;
  getSession(authToken?: string): Promise<SessionInfo>;
  getTables(): Promise<Table[]>;
  joinTable(tableId: string): Promise<{ success: boolean; message?: string }>;
  leaveTable(tableId: string): Promise<{ success: boolean }>;
  getTableState(tableId: string): Promise<GameState>;
  getLatestReplay(tableId: string): Promise<LatestReplay>;
  getTableHands(tableId: string): Promise<HandSummary[]>;
  getHandActions(handId: string): Promise<string[]>;
  getHandReplay(handId: string): Promise<HandReplay>;
  getTableLive(tableId: string, afterAction?: number): Promise<TableLive>;
  submitAction(tableId: string, action: ActionRequest): Promise<GameState>;
}
