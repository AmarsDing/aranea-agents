import axios from 'axios';
import { i18n } from '../../i18n';

/** Maps axios / network errors to a user-visible knowledge-page message. */
export function knowledgePageError(err: unknown): { message: string; unavailable: string } {
  if (axios.isAxiosError(err)) {
    const status = err.response?.status;
    if (status === 404) {
      return {
        message: i18n.global.t(
          'knowledgePage.errorApiNotFound',
          'Knowledge API not found (404). Make sure the backend has been restarted and the routes are registered.',
        ),
        unavailable: '',
      };
    }
    if (!err.response && (err.code === 'ERR_NETWORK' || err.message === 'Network Error')) {
      return {
        message: i18n.global.t(
          'knowledgePage.errorNetworkUnavailable',
          'Cannot reach the backend. Make sure the backend service is running.',
        ),
        unavailable: '',
      };
    }
    const data = err.response?.data as { message?: string } | undefined;
    if (typeof data?.message === 'string' && data.message.trim()) {
      return { message: data.message, unavailable: '' };
    }
  }
  const msg = err instanceof Error ? err.message : String(err);
  if (/unavailable|pgvector|postgres|not configured/i.test(msg)) {
    return { message: '', unavailable: msg };
  }
  return { message: msg, unavailable: '' };
}
