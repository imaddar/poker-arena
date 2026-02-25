import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { createHttpApiClient } from '../src/api/client.ts';

describe('createHttpApiClient', () => {
  it('maps GET /tables snake_case payload into UI table model', async () => {
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(
        JSON.stringify([
          {
            id: 'table-1',
            name: 'Alpha',
            max_seats: 6,
            small_blind: 50,
            big_blind: 100,
            status: 'running',
          },
        ]),
        { status: 200 },
      );

    try {
      const api = createHttpApiClient({
        baseUrl: 'http://127.0.0.1:8080',
        getToken: () => 'admin-token',
      });

      const tables = await api.getTables();
      assert.equal(tables.length, 1);
      assert.equal(tables[0].id, 'table-1');
      assert.equal(tables[0].status, 'playing');
      assert.equal(tables[0].bigBlind, 100);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  it('requires a token during login for real API mode', async () => {
    const api = createHttpApiClient({
      baseUrl: 'http://127.0.0.1:8080',
      getToken: () => '',
    });

    await assert.rejects(() => api.login('operator'), /Missing auth token/);
  });

  it('uses explicit login token when default token is empty', async () => {
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response(JSON.stringify({ role: 'seat', seat_no: 1 }), { status: 200 });

    const api = createHttpApiClient({
      baseUrl: 'http://127.0.0.1:8080',
      getToken: () => '',
    });

    try {
      const user = await api.login('observer', 'seat-1-token');
      assert.equal(user.token, 'seat-1-token');
      assert.equal(user.role, 'observer');
      assert.equal(user.seatNo, 1);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  it('uses latest token from token provider for authenticated requests', async () => {
    const originalFetch = globalThis.fetch;
    let activeToken = 'admin-token';
    let lastAuthHeader = '';
    globalThis.fetch = async (_input, init) => {
      const headers = new Headers(init?.headers);
      lastAuthHeader = headers.get('Authorization') ?? '';
      return new Response('[]', { status: 200 });
    };

    try {
      const api = createHttpApiClient({
        baseUrl: 'http://127.0.0.1:8080',
        getToken: () => activeToken,
      });

      await api.getTables();
      assert.equal(lastAuthHeader, 'Bearer admin-token');

      activeToken = 'seat-1-token';
      await api.getTables();
      assert.equal(lastAuthHeader, 'Bearer seat-1-token');
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  it('maps table state payload into a game state model', async () => {
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(
        JSON.stringify({
          table: {
            id: 'table-2',
            name: 'Beta',
            max_seats: 6,
            small_blind: 100,
            big_blind: 200,
            status: 'idle',
          },
          seats: [
            {
              seat_no: 1,
              stack: 10000,
              status: 'active',
            },
          ],
        }),
        { status: 200 },
      );

    try {
      const api = createHttpApiClient({
        baseUrl: 'http://127.0.0.1:8080',
        getToken: () => 'admin-token',
      });

      const state = await api.getTableState('table-2');
      assert.equal(state.tableId, 'table-2');
      assert.equal(state.minRaise, 200);
      assert.equal(state.seats[0]?.stack, 10000);
      assert.equal(state.seats[0]?.status, 'active');
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  it('maps hands and actions history from backend endpoints', async () => {
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async (input) => {
      const url = String(input);
      if (url.endsWith('/tables/table-2/hands')) {
        return new Response(
          JSON.stringify([
            {
              hand_id: 'hand-22',
              hand_no: 22,
            },
          ]),
          { status: 200 },
        );
      }
      if (url.endsWith('/hands/hand-22/actions')) {
        return new Response(
          JSON.stringify([
            {
              street: 'turn',
              acting_seat: 2,
              action: 'call',
              amount: 150,
              is_fallback: false,
            },
          ]),
          { status: 200 },
        );
      }
      return new Response('[]', { status: 200 });
    };

    try {
      const api = createHttpApiClient({
        baseUrl: 'http://127.0.0.1:8080',
        getToken: () => 'admin-token',
      });
      const hands = await api.getTableHands('table-2');
      assert.equal(hands[0].handId, 'hand-22');

      const actions = await api.getHandActions('hand-22');
      assert.equal(actions[0], 'TURN S2: CALL 150');
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  it('maps latest replay payload from table replay endpoint', async () => {
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(
        JSON.stringify({
          latest_hand: {
            hand_id: 'hand-77',
            hand_no: 77,
          },
          replay: {
            actions: [
              {
                street: 'river',
                acting_seat: 1,
                action: 'raise',
                amount: 400,
                is_fallback: true,
              },
            ],
            analytics: {
              total_actions: 1,
              fallback_actions: 1,
            },
          },
        }),
        { status: 200 },
      );

    try {
      const api = createHttpApiClient({
        baseUrl: 'http://127.0.0.1:8080',
        getToken: () => 'admin-token',
      });
      const latest = await api.getLatestReplay('table-3');
      assert.equal(latest.handId, 'hand-77');
      assert.equal(latest.handNo, 77);
      assert.equal(latest.totalActions, 1);
      assert.equal(latest.fallbackActions, 1);
      assert.equal(latest.actionLog[0], 'RIVER S1: RAISE 400 (fallback)');
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  it('maps /session role payload into frontend session info', async () => {
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response(JSON.stringify({ role: 'admin' }), { status: 200 });

    try {
      const api = createHttpApiClient({
        baseUrl: 'http://127.0.0.1:8080',
        getToken: () => 'admin-token',
      });
      const session = await api.getSession();
      assert.equal(session.role, 'admin');
      assert.equal(session.seatNo, undefined);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  it('maps hand replay payload into board, hole cards, and action decisions', async () => {
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(
        JSON.stringify({
          hand_id: 'hand-101',
          final_state: {
            board: [
              { rank: 14, suit: 'spades' },
              { rank: 13, suit: 'hearts' },
            ],
            hole_cards: [
              {
                seat_no: 1,
                cards: [
                  { rank: 12, suit: 'diamonds' },
                  { rank: 11, suit: 'clubs' },
                ],
              },
            ],
          },
          actions: [
            {
              street: 'preflop',
              acting_seat: 1,
              action: 'raise',
              amount: 300,
              is_fallback: false,
            },
          ],
        }),
        { status: 200 },
      );

    try {
      const api = createHttpApiClient({
        baseUrl: 'http://127.0.0.1:8080',
        getToken: () => 'admin-token',
      });
      const replay = await api.getHandReplay('hand-101');
      assert.equal(replay.handId, 'hand-101');
      assert.equal(replay.board[0].rank, 'A');
      assert.equal(replay.board[0].suit, 's');
      assert.equal(replay.holeCardsBySeat[1][0].rank, 'Q');
      assert.equal(replay.holeCardsBySeat[1][1].rank, 'J');
      assert.equal(replay.actions[0].actingSeat, 1);
      assert.equal(replay.actions[0].action, 'raise');
      assert.equal(replay.actions[0].amount, 300);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });
});
