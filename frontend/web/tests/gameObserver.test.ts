import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { applyObserverReplayToState } from '../src/pages/gameObserver.ts';
import type { GameState } from '../src/types/index.ts';

function baseState(): GameState {
  return {
    tableId: 'table-1',
    handId: 'seed-hand',
    street: 'preflop',
    communityCards: [],
    pot: 0,
    currentTurnSeat: -1,
    dealerSeat: 0,
    seats: [null, null, null, null, null, null],
    toCall: 0,
    minRaise: 100,
    actionLog: ['seed-log'],
  };
}

describe('applyObserverReplayToState', () => {
  it('shows no-history message when latest replay has no hand', () => {
    const next = applyObserverReplayToState(baseState(), { actionLog: [] });
    assert.equal(next.handId, 'seed-hand');
    assert.deepEqual(next.actionLog, ['No hands recorded for this table yet.']);
  });

  it('applies latest hand id and action log when replay exists', () => {
    const next = applyObserverReplayToState(baseState(), {
      handId: 'hand-77',
      actionLog: ['RIVER S1: RAISE 400 (fallback)'],
    });
    assert.equal(next.handId, 'hand-77');
    assert.deepEqual(next.actionLog, ['RIVER S1: RAISE 400 (fallback)']);
  });
});
