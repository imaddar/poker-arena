import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { loadGameState } from '../src/pages/gameLoader.ts';
import type { ApiClient, LatestReplay } from '../src/api/types.ts';
import type { GameState, Table, User } from '../src/types/index.ts';

function makeState(): GameState {
  return {
    tableId: 'table-1',
    tableName: 'Table 1',
    handId: 'seed-hand',
    street: 'preflop',
    communityCards: [],
    pot: 0,
    currentTurnSeat: -1,
    dealerSeat: 0,
    seats: [null, null, { id: 'hero', name: 'You', seat: 2, stack: 1200, bet: 0, status: 'active', isDealer: false, isTurn: false }, null, null, null],
    toCall: 0,
    minRaise: 200,
    actionLog: ['seed-log'],
  };
}

function makeApi(replay: LatestReplay): ApiClient {
  const state = makeState();
  return {
    async login(_username: string): Promise<User> {
      throw new Error('unused in test');
    },
    async getTables(): Promise<Table[]> {
      throw new Error('unused in test');
    },
    async joinTable(_tableId: string): Promise<{ success: boolean; message?: string }> {
      throw new Error('unused in test');
    },
    async leaveTable(_tableId: string): Promise<{ success: boolean }> {
      throw new Error('unused in test');
    },
    async getTableState(_tableId: string): Promise<GameState> {
      return state;
    },
    async getLatestReplay(_tableId: string): Promise<LatestReplay> {
      return replay;
    },
    async getTableHands(_tableId: string) {
      return [];
    },
    async getHandActions(_handId: string) {
      return [];
    },
    async submitAction(_tableId: string) {
      return state;
    },
  };
}

describe('loadGameState', () => {
  it('applies latest replay action log for backend observer mode', async () => {
    const api = makeApi({
      handId: 'hand-77',
      actionLog: ['RIVER S1: RAISE 400 (fallback)'],
    });
    const loaded = await loadGameState(api, 'table-1', false);
    assert.equal(loaded.state.handId, 'hand-77');
    assert.deepEqual(loaded.state.actionLog, ['RIVER S1: RAISE 400 (fallback)']);
    assert.equal(loaded.raiseAmount, 200);
  });

  it('shows no-history message when backend replay has no hand', async () => {
    const api = makeApi({ actionLog: [] });
    const loaded = await loadGameState(api, 'table-1', false);
    assert.equal(loaded.state.handId, 'seed-hand');
    assert.deepEqual(loaded.state.actionLog, ['No hands recorded for this table yet.']);
  });
});
