import { createHttpApiClient } from './client';
import { resolveApiRuntimeConfig } from './config';
import { createMockApi } from './mock';
import type { ApiClient } from './types';

const cfg = resolveApiRuntimeConfig(import.meta.env);

export const api: ApiClient = cfg.useMock
  ? createMockApi()
  : createHttpApiClient({
      baseUrl: cfg.baseUrl,
      getToken: () => cfg.adminToken,
    });
