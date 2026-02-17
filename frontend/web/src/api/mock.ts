import type { ActionRequest, Card, GameState, Table, User } from '../types';
import type { ApiClient } from './types';

const HERO_SEAT = 2;
const STREET_CARD_COUNT: Record<GameState['street'], number> = {
  preflop: 0,
  flop: 3,
  turn: 4,
  river: 5,
};

const delay = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

function randomCard(): Card {
  const ranks: Card['rank'][] = ['2', '3', '4', '5', '6', '7', '8', '9', 'T', 'J', 'Q', 'K', 'A'];
  const suits: Card['suit'][] = ['s', 'h', 'd', 'c'];

  return {
    rank: ranks[Math.floor(Math.random() * ranks.length)],
    suit: suits[Math.floor(Math.random() * suits.length)],
  };
}

function makeActionLog(base: string): string[] {
  return [
    `${base}: blinds posted`,
    'Dealer: cards are in the air',
    'You are on the button this hand',
  ];
}

function createInitialState(tableId: string, tableName?: string): GameState {
  return {
    tableId,
    tableName,
    handId: `h-${Math.floor(Math.random() * 100000)}`,
    street: 'preflop',
    communityCards: [],
    pot: 150,
    currentTurnSeat: HERO_SEAT,
    dealerSeat: 0,
    seats: [
      {
        id: `${tableId}-a1`,
        name: 'Nova',
        seat: 0,
        stack: 9500,
        bet: 100,
        status: 'active',
        isDealer: true,
        isTurn: false,
        holeCards: [randomCard(), randomCard()],
      },
      {
        id: `${tableId}-a2`,
        name: 'Rhea',
        seat: 1,
        stack: 8100,
        bet: 50,
        status: 'active',
        isDealer: false,
        isTurn: false,
        holeCards: [randomCard(), randomCard()],
      },
      {
        id: 'hero',
        name: 'You',
        seat: HERO_SEAT,
        stack: 10000,
        bet: 0,
        status: 'active',
        isDealer: false,
        isTurn: true,
        holeCards: [randomCard(), randomCard()],
      },
      null,
      {
        id: `${tableId}-a3`,
        name: 'Taro',
        seat: 4,
        stack: 7200,
        bet: 0,
        status: 'active',
        isDealer: false,
        isTurn: false,
        holeCards: [randomCard(), randomCard()],
      },
      {
        id: `${tableId}-a4`,
        name: 'Iris',
        seat: 5,
        stack: 6400,
        bet: 0,
        status: 'active',
        isDealer: false,
        isTurn: false,
        holeCards: [randomCard(), randomCard()],
      },
    ],
    toCall: 100,
    minRaise: 200,
    actionLog: makeActionLog(`Hand ${tableId.toUpperCase()}`),
  };
}

function cloneGameState(state: GameState): GameState {
  return {
    ...state,
    communityCards: [...state.communityCards],
    seats: state.seats.map((seat) => (seat ? { ...seat, holeCards: seat.holeCards ? [...seat.holeCards] : undefined } : null)),
    actionLog: [...state.actionLog],
  };
}

function advanceStreet(state: GameState): void {
  const order: GameState['street'][] = ['preflop', 'flop', 'turn', 'river'];
  const currentIndex = order.indexOf(state.street);
  const nextStreet = order[Math.min(currentIndex + 1, order.length - 1)];

  state.street = nextStreet;
  const cardsNeeded = STREET_CARD_COUNT[nextStreet];
  while (state.communityCards.length < cardsNeeded) {
    state.communityCards.push(randomCard());
  }

  state.toCall = 0;
  state.minRaise = Math.max(100, Math.floor(state.pot * 0.25));
  for (const seat of state.seats) {
    if (seat) {
      seat.bet = 0;
      seat.isTurn = seat.seat === HERO_SEAT;
    }
  }

  state.currentTurnSeat = HERO_SEAT;
  state.actionLog.unshift(`Board advanced to ${nextStreet.toUpperCase()}`);
}

class MockApi implements ApiClient {
  private tables: Table[] = [
    { id: 't1', name: 'Mercury', maxSeats: 6, smallBlind: 25, bigBlind: 50, status: 'playing', players: 4 },
    { id: 't2', name: 'Vega', maxSeats: 6, smallBlind: 50, bigBlind: 100, status: 'waiting', players: 2 },
    { id: 't3', name: 'Atlas', maxSeats: 6, smallBlind: 100, bigBlind: 200, status: 'playing', players: 5 },
    { id: 't4', name: 'Cinder', maxSeats: 6, smallBlind: 200, bigBlind: 400, status: 'waiting', players: 1 },
    { id: 't5', name: 'Drift', maxSeats: 6, smallBlind: 10, bigBlind: 20, status: 'waiting', players: 3 },
  ];

  private tableState = new Map<string, GameState>();

  async login(username: string): Promise<User> {
    await delay(250);

    return {
      id: `u-${Date.now()}`,
      name: username.trim(),
      token: `tok-${Date.now()}`,
    };
  }

  async getTables(): Promise<Table[]> {
    await delay(200);
    return [...this.tables];
  }

  async joinTable(tableId: string): Promise<{ success: boolean; message?: string }> {
    await delay(150);
    const table = this.tables.find((entry) => entry.id === tableId);

    if (!table) {
      return { success: false, message: 'Table not found.' };
    }

    if (table.players >= table.maxSeats) {
      return { success: false, message: 'Table is full.' };
    }

    if (table.status === 'waiting') {
      table.status = 'playing';
    }

    table.players += 1;

    if (!this.tableState.has(tableId)) {
      this.tableState.set(tableId, createInitialState(tableId, table.name));
    }

    return { success: true };
  }

  async leaveTable(tableId: string): Promise<{ success: boolean }> {
    await delay(100);
    const table = this.tables.find((entry) => entry.id === tableId);

    if (table) {
      table.players = Math.max(0, table.players - 1);
      if (table.players <= 1) {
        table.status = 'waiting';
      }
    }

    return { success: true };
  }

  async getTableState(tableId: string): Promise<GameState> {
    await delay(180);

    const table = this.tables.find((entry) => entry.id === tableId);
    const existing = this.tableState.get(tableId) ?? createInitialState(tableId, table?.name);
    this.tableState.set(tableId, existing);

    const state = cloneGameState(existing);
    if (state.currentTurnSeat !== HERO_SEAT) {
      state.currentTurnSeat = HERO_SEAT;
      for (const seat of state.seats) {
        if (seat) {
          seat.isTurn = seat.seat === HERO_SEAT;
        }
      }
    }

    return state;
  }

  async submitAction(tableId: string, action: ActionRequest): Promise<GameState> {
    await delay(220);

    const table = this.tables.find((entry) => entry.id === tableId);
    const existing = this.tableState.get(tableId) ?? createInitialState(tableId, table?.name);
    const state = cloneGameState(existing);
    const hero = state.seats[HERO_SEAT];

    if (!hero || hero.status !== 'active') {
      return state;
    }

    const wager = Math.max(0, action.amount ?? 0);

    if (action.type === 'fold') {
      hero.status = 'folded';
      hero.isTurn = false;
      state.currentTurnSeat = 0;
      state.actionLog.unshift('You folded');
    }

    if (action.type === 'check') {
      state.actionLog.unshift('You checked');
    }

    if (action.type === 'call') {
      const callAmount = Math.min(state.toCall, hero.stack);
      hero.stack -= callAmount;
      hero.bet += callAmount;
      state.pot += callAmount;
      state.toCall = 0;
      state.actionLog.unshift(`You called ${callAmount}`);
    }

    if (action.type === 'bet' || action.type === 'raise') {
      const amount = Math.min(hero.stack, Math.max(state.minRaise, wager));
      hero.stack -= amount;
      hero.bet += amount;
      state.pot += amount;
      state.toCall = Math.max(0, amount - 100);
      state.minRaise = Math.max(state.minRaise, amount + 100);
      state.actionLog.unshift(`You ${action.type}d to ${amount}`);
    }

    if (hero.stack === 0) {
      hero.status = 'allin';
      state.actionLog.unshift('You are all in');
    }

    if (hero.status === 'active' || hero.status === 'allin') {
      advanceStreet(state);
    }

    this.tableState.set(tableId, state);
    return cloneGameState(state);
  }
}

export function createMockApi(): ApiClient {
  return new MockApi();
}
