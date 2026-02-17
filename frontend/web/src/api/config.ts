export interface ApiRuntimeConfig {
  useMock: boolean;
  baseUrl: string;
  adminToken: string;
}

export type ApiEnv = Record<string, unknown>;

function parseBooleanEnv(raw: string | undefined, defaultValue: boolean): boolean {
  if (raw == null) {
    return defaultValue;
  }
  const normalized = raw.trim().toLowerCase();
  if (normalized === '') {
    return defaultValue;
  }
  if (normalized === 'true' || normalized === '1' || normalized === 'yes' || normalized === 'on') {
    return true;
  }
  if (normalized === 'false' || normalized === '0' || normalized === 'no' || normalized === 'off') {
    return false;
  }
  return defaultValue;
}

export function resolveApiRuntimeConfig(env: ApiEnv): ApiRuntimeConfig {
  const useMockRaw = typeof env.VITE_USE_MOCK_API === 'string' ? env.VITE_USE_MOCK_API : undefined;
  const baseUrl = typeof env.VITE_API_BASE_URL === 'string' ? env.VITE_API_BASE_URL : '';
  const adminToken = typeof env.VITE_ADMIN_TOKEN === 'string' ? env.VITE_ADMIN_TOKEN : '';

  return {
    // Safe default for local development is mock mode.
    useMock: parseBooleanEnv(useMockRaw, true),
    baseUrl,
    adminToken,
  };
}
