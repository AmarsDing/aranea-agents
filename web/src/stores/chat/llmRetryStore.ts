/**
 * LLM provider alert store — tracks per-session banners for:
 *   retry      llm_retry      network reconnect (transient)
 *   rate_limit llm_retry      429 backoff (transient)
 *   billing    llm_billing    account arrears (durable until next turn / dismiss)
 *   auth       llm_auth       credential failure (durable)
 *   stall      llm_stall      first-byte silence (durable)
 */
import { ref } from 'vue';
import { defineStore } from 'pinia';

export type LlmAlertKind = 'retry' | 'billing' | 'auth' | 'stall' | 'rate_limit';

export interface LlmRetryState {
  kind: LlmAlertKind;
  attempt: number;
  /** -1 means infinite retries (backend default). */
  maxRetries: number;
  delayMs: number;
  error: string;
  message: string;
  updatedAt: number;
}

/** Meta payload carried by llm_retry / llm_billing / llm_auth / llm_stall notices. */
export interface LlmRetryMeta {
  kind?: unknown;
  attempt?: unknown;
  max_retries?: unknown;
  delay_ms?: unknown;
  error?: unknown;
  message?: unknown;
}

const TRANSIENT_KINDS: ReadonlySet<LlmAlertKind> = new Set(['retry', 'rate_limit']);

function toInt(v: unknown, fallback: number): number {
  const n = typeof v === 'number' ? v : typeof v === 'string' ? Number(v) : NaN;
  return Number.isFinite(n) ? Math.trunc(n) : fallback;
}

function asKind(v: unknown, fallback: LlmAlertKind): LlmAlertKind {
  if (v === 'retry' || v === 'billing' || v === 'auth' || v === 'stall' || v === 'rate_limit') {
    return v;
  }
  return fallback;
}

export const useLlmRetryStore = defineStore('llmRetry', () => {
  const retries = ref<Record<string, LlmRetryState>>({});

  function noteRetry(sessionId: string, meta: LlmRetryMeta) {
    const sid = sessionId.trim();
    if (!sid) return;
    retries.value[sid] = {
      kind: asKind(meta.kind, 'retry'),
      attempt: Math.max(1, toInt(meta.attempt, 1)),
      maxRetries: toInt(meta.max_retries, -1),
      delayMs: Math.max(0, toInt(meta.delay_ms, 0)),
      error: typeof meta.error === 'string' ? meta.error : '',
      message: typeof meta.message === 'string' ? meta.message : '',
      updatedAt: Date.now(),
    };
  }

  function noteAlert(sessionId: string, kind: LlmAlertKind, meta: LlmRetryMeta = {}) {
    const sid = sessionId.trim();
    if (!sid) return;
    retries.value[sid] = {
      kind,
      attempt: Math.max(1, toInt(meta.attempt, 1)),
      maxRetries: toInt(meta.max_retries, 0),
      delayMs: Math.max(0, toInt(meta.delay_ms, 0)),
      error: typeof meta.error === 'string' ? meta.error : '',
      message: typeof meta.message === 'string' ? meta.message : '',
      updatedAt: Date.now(),
    };
  }

  function clear(sessionId: string) {
    if (!sessionId) return;
    delete retries.value[sessionId];
  }

  /** Clear reconnect/rate-limit banners only; keep billing/auth/stall until the next turn. */
  function clearTransient(sessionId: string) {
    if (!sessionId) return;
    const state = retries.value[sessionId];
    if (state && TRANSIENT_KINDS.has(state.kind)) {
      delete retries.value[sessionId];
    }
  }

  function clearAll() {
    retries.value = {};
  }

  function retryFor(sessionId: string): LlmRetryState | null {
    if (!sessionId) return null;
    return retries.value[sessionId] ?? null;
  }

  return { retries, noteRetry, noteAlert, clear, clearTransient, clearAll, retryFor };
});
