import { createHttpApiClient } from './client';
import { resolveApiRuntimeConfig } from './config';
import { createMockApi } from './mock';
import type { ApiClient } from './types';

const cfg = resolveApiRuntimeConfig(import.meta.env);
let runtimeToken = cfg.adminToken.trim();

export function getApiAuthToken(): string {
  return runtimeToken;
}

export function setApiAuthToken(token: string): void {
  runtimeToken = token.trim();
}

export function clearApiAuthToken(): void {
  runtimeToken = '';
}

export const api: ApiClient = cfg.useMock
  ? createMockApi()
  : createHttpApiClient({
      baseUrl: cfg.baseUrl,
      getToken: () => runtimeToken,
    });
