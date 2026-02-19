import type { ActionRequest, GameState, Table, User } from '../types';

export interface HandSummary {
  handId: string;
  handNo: number;
  endedAt?: string;
}

export interface LatestReplay {
  handId?: string;
  actionLog: string[];
}

export interface ApiClient {
  login(username: string): Promise<User>;
  getTables(): Promise<Table[]>;
  joinTable(tableId: string): Promise<{ success: boolean; message?: string }>;
  leaveTable(tableId: string): Promise<{ success: boolean }>;
  getTableState(tableId: string): Promise<GameState>;
  getLatestReplay(tableId: string): Promise<LatestReplay>;
  getTableHands(tableId: string): Promise<HandSummary[]>;
  getHandActions(handId: string): Promise<string[]>;
  submitAction(tableId: string, action: ActionRequest): Promise<GameState>;
}
