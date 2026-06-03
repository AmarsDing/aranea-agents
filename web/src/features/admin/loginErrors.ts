import type { AxiosError } from 'axios';

export type LoginErrorKind = 'network' | 'credentials' | 'server' | 'unknown';

export type LoginErrorInfo = {
  kind: LoginErrorKind;
  message: string;
};

function kratosMessage(data: unknown): string | undefined {
  if (!data || typeof data !== 'object') return undefined;
  const msg = (data as Record<string, unknown>).message;
  return typeof msg === 'string' && msg.trim() ? msg.trim() : undefined;
}

/** Map login API failures to user-actionable messages (design: admin-auth §5). */
export function formatLoginError(err: unknown, t: (key: string) => string): LoginErrorInfo {
  const axiosErr = err as AxiosError | undefined;
  if (!axiosErr?.isAxiosError) {
    return {
      kind: 'unknown',
      message: t('auth.loginFailedUnknown'),
    };
  }

  if (!axiosErr.response) {
    return {
      kind: 'network',
      message: t('auth.loginFailedNetwork'),
    };
  }

  const status = axiosErr.response.status;
  const apiMsg = kratosMessage(axiosErr.response.data);

  if (status === 401 || status === 400) {
    return {
      kind: 'credentials',
      message: apiMsg || t('auth.loginFailedCredentials'),
    };
  }

  if (status >= 500) {
    return {
      kind: 'server',
      message: apiMsg || t('auth.loginFailedServer'),
    };
  }

  return {
    kind: 'unknown',
    message: apiMsg || t('auth.loginFailed'),
  };
}
