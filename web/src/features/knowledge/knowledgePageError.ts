import axios from 'axios';

/** Maps axios / network errors to a user-visible knowledge-page message. */
export function knowledgePageError(err: unknown): { message: string; unavailable: string } {
  if (axios.isAxiosError(err)) {
    const status = err.response?.status;
    if (status === 404) {
      return { message: '知识库 API 未找到 (404)。请确认后端已重启且路由已注册。', unavailable: '' };
    }
    if (!err.response && (err.code === 'ERR_NETWORK' || err.message === 'Network Error')) {
      return { message: '无法连接后端，请确认后端服务已启动。', unavailable: '' };
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
