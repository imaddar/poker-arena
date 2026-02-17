import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { resolveApiRuntimeConfig } from '../src/api/config.ts';

describe('resolveApiRuntimeConfig', () => {
  it('defaults to mock mode when no env vars are provided', () => {
    const cfg = resolveApiRuntimeConfig({});
    assert.equal(cfg.useMock, true);
    assert.equal(cfg.baseUrl, '');
    assert.equal(cfg.adminToken, '');
  });

  it('supports explicit non-mock mode', () => {
    const cfg = resolveApiRuntimeConfig({
      VITE_USE_MOCK_API: 'false',
      VITE_API_BASE_URL: 'http://127.0.0.1:8080',
      VITE_ADMIN_TOKEN: 'local-admin-token',
    });
    assert.equal(cfg.useMock, false);
    assert.equal(cfg.baseUrl, 'http://127.0.0.1:8080');
    assert.equal(cfg.adminToken, 'local-admin-token');
  });

  it('accepts flexible boolean forms', () => {
    assert.equal(resolveApiRuntimeConfig({ VITE_USE_MOCK_API: '1' }).useMock, true);
    assert.equal(resolveApiRuntimeConfig({ VITE_USE_MOCK_API: 'on' }).useMock, true);
    assert.equal(resolveApiRuntimeConfig({ VITE_USE_MOCK_API: '0' }).useMock, false);
    assert.equal(resolveApiRuntimeConfig({ VITE_USE_MOCK_API: 'off' }).useMock, false);
  });
});
