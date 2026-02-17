import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { clampRaiseAmount, filterTables, getQuickBetAmount } from '../src/lib/pokerLogic.ts';

describe('clampRaiseAmount', () => {
  it('keeps raise between min raise and stack', () => {
    assert.equal(clampRaiseAmount({ requested: 700, minRaise: 400, stack: 1200 }), 700);
    assert.equal(clampRaiseAmount({ requested: 100, minRaise: 400, stack: 1200 }), 400);
    assert.equal(clampRaiseAmount({ requested: 4000, minRaise: 400, stack: 1200 }), 1200);
  });

  it('returns stack when stack is below minimum raise', () => {
    assert.equal(clampRaiseAmount({ requested: 500, minRaise: 1000, stack: 700 }), 700);
  });
});

describe('getQuickBetAmount', () => {
  it('calculates quick bet presets and clamps to legal range', () => {
    assert.equal(getQuickBetAmount({ preset: 'min', minRaise: 300, toCall: 100, pot: 1000, stack: 2000 }), 300);
    assert.equal(getQuickBetAmount({ preset: 'half-pot', minRaise: 300, toCall: 100, pot: 1000, stack: 2000 }), 500);
    assert.equal(getQuickBetAmount({ preset: 'pot', minRaise: 300, toCall: 100, pot: 1000, stack: 2000 }), 1000);
    assert.equal(getQuickBetAmount({ preset: 'max', minRaise: 300, toCall: 100, pot: 1000, stack: 2000 }), 2000);
  });
});

describe('filterTables', () => {
  it('applies status, stake, and search filters', () => {
    const tables = [
      { id: 't1', name: 'Mercury', maxSeats: 6, smallBlind: 25, bigBlind: 50, status: 'waiting', players: 2 },
      { id: 't2', name: 'Saturn', maxSeats: 6, smallBlind: 100, bigBlind: 200, status: 'playing', players: 6 },
      { id: 't3', name: 'Comet', maxSeats: 6, smallBlind: 10, bigBlind: 20, status: 'waiting', players: 4 },
    ] as const;

    const filtered = filterTables({
      tables: [...tables],
      filters: { status: 'waiting', stake: 'low', query: 'mer' },
    });

    assert.equal(filtered.length, 1);
    assert.equal(filtered[0].id, 't1');
  });
});
