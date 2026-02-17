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

  it('requires an admin token during login for real API mode', async () => {
    const api = createHttpApiClient({
      baseUrl: 'http://127.0.0.1:8080',
      getToken: () => '',
    });

    await assert.rejects(() => api.login('operator'), /Missing VITE_ADMIN_TOKEN/);
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
});
