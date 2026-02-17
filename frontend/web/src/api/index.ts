import { createHttpApiClient } from './client';
import { createMockApi } from './mock';
import type { ApiClient } from './types';

const useMock = String(import.meta.env.VITE_USE_MOCK_API ?? '').toLowerCase() === 'true';
const adminToken = String(import.meta.env.VITE_ADMIN_TOKEN ?? '');
const baseUrl = String(import.meta.env.VITE_API_BASE_URL ?? '');

export const api: ApiClient = useMock
  ? createMockApi()
  : createHttpApiClient({
      baseUrl,
      getToken: () => adminToken,
    });
