export interface User {
  id: string;
  name: string;
  token: string;
}

export interface Table {
  id: string;
  name: string;
  maxSeats: number;
  smallBlind: number;
  bigBlind: number;
  status: 'waiting' | 'playing' | 'completed';
  players: number;
}

export interface Card {
  rank: string;
  suit: 's' | 'h' | 'd' | 'c';
}

export type ActionType = 'fold' | 'check' | 'call' | 'bet' | 'raise';

export interface ActionRequest {
  type: ActionType;
  amount?: number;
}

export interface Player {
  id: string;
  name: string;
  seat: number;
  stack: number;
  bet: number;
  status: 'active' | 'folded' | 'allin' | 'sitting_out';
  holeCards?: Card[];
  isDealer: boolean;
  isTurn: boolean;
}

export interface GameState {
  tableId: string;
  tableName?: string;
  handId: string;
  street: 'preflop' | 'flop' | 'turn' | 'river';
  communityCards: Card[];
  pot: number;
  currentTurnSeat: number;
  dealerSeat: number;
  seats: (Player | null)[];
  toCall: number;
  minRaise: number;
  actionLog: string[];
}
