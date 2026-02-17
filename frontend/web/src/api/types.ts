import type { ActionRequest, GameState, Table, User } from '../types';

export interface ApiClient {
  login(username: string): Promise<User>;
  getTables(): Promise<Table[]>;
  joinTable(tableId: string): Promise<{ success: boolean; message?: string }>;
  leaveTable(tableId: string): Promise<{ success: boolean }>;
  getTableState(tableId: string): Promise<GameState>;
  submitAction(tableId: string, action: ActionRequest): Promise<GameState>;
}
