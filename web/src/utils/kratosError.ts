import axios from 'axios';

export type KratosApiError = {
  status?: number;
  reason?: string;
  message: string;
};

export type AgentCreateFieldErrors = {
  agent_key?: string;
  display_name?: string;
  provider?: string;
  model?: string;
  remote_url?: string;
  form?: string;
};

type ErrorEnvelope = {
  status?: number;
  reason?: string;
  message?: string;
};

/**
 * 从错误信封 `{code, reason, message}` 提取状态/原因。
 * `code` 兼容数字 403 与字符串 `PERMISSION_DENIED`（部分包装层只保 code 不保 reason）。
 */
function readEnvelope(value: unknown): ErrorEnvelope {
  if (!value || typeof value !== 'object') return {};
  const record = value as Record<string, unknown>;
  const data =
    record.response && typeof record.response === 'object'
      ? ((record.response as Record<string, unknown>).data as Record<string, unknown> | undefined)
      : undefined;
  const source = data && typeof data === 'object' ? data : record;
  const code = source.code;
  const reason = typeof source.reason === 'string' ? source.reason : typeof code === 'string' ? code : undefined;
  return {
    status: typeof record.status === 'number' ? record.status : typeof code === 'number' ? code : undefined,
    reason,
    message: typeof source.message === 'string' ? source.message : undefined,
  };
}

export function parseKratosApiError(err: unknown): KratosApiError {
  if (axios.isAxiosError(err)) {
    const data = err.response?.data as Record<string, unknown> | undefined;
    const envelope = readEnvelope(data);
    const message = typeof data?.message === 'string' ? data.message : err.message || '请求失败';
    return {
      status: err.response?.status ?? envelope.status,
      reason: typeof data?.reason === 'string' ? data.reason : envelope.reason,
      message,
    };
  }
  // 兼容已被包装过一次的错误：自身带 code/response 信封，或把原始错误放在 cause 上。
  const direct = readEnvelope(err);
  if (direct.status !== undefined || direct.reason) {
    return {
      status: direct.status,
      reason: direct.reason,
      message: direct.message ?? (err instanceof Error ? err.message : '请求失败'),
    };
  }
  if (err instanceof Error) {
    const cause = (err as Error & { cause?: unknown }).cause;
    if (cause !== undefined && cause !== err) {
      const nested = parseKratosApiError(cause);
      if (nested.status !== undefined || nested.reason) {
        return { ...nested, message: err.message || nested.message };
      }
    }
    return { message: err.message };
  }
  const nestedCause = (err as Record<string, unknown> | null)?.cause;
  if (nestedCause !== undefined && nestedCause !== err) {
    const nested = parseKratosApiError(nestedCause);
    if (nested.status !== undefined || nested.reason) return nested;
  }
  return { message: '请求失败' };
}

/** 403（无权限）判定：列表页据此渲染 forbidden 态而非可重试错误态。 */
export function isForbiddenApiError(err: unknown): boolean {
  const { status, reason } = parseKratosApiError(err);
  return status === 403 || reason === 'PERMISSION_DENIED' || reason === 'FORBIDDEN';
}

export function mapAgentCreateFieldErrors(err: KratosApiError): AgentCreateFieldErrors {
  const msg = err.message;
  switch (err.reason) {
    case 'AGENT_KEY_INVALID':
    case 'AGENT_KEY_CONFLICT':
      return { agent_key: msg };
    case 'AGENT':
      if (/agent_key/i.test(msg)) return { agent_key: msg };
      if (/display_name/i.test(msg)) return { display_name: msg };
      if (/remote_url|a2a_proxy/i.test(msg)) return { remote_url: msg };
      if (/provider|model/i.test(msg)) return { provider: msg, model: msg };
      return { form: msg };
    default:
      return { form: msg };
  }
}
