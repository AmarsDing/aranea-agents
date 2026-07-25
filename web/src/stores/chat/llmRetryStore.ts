/**
 * LLM retry status store — tracks transient "llm_retry" system notices per
 * chat session so the UI can show a reconnect banner while the provider
 * retry transport is backing off. State is cleared when the stream resumes
 * (step.streaming), a new turn starts, or the run reaches a terminal status.
 */
import { ref } from 'vue';
import { defineStore } from 'pinia';

export interface LlmRetryState {
  attempt: number;
  /** -1 means infinite retries (backend default). */
  maxRetries: number;
  delayMs: number;
  error: string;
  updatedAt: number;
}

/** Meta payload carried by the llm_retry system.notice event. */
export interface LlmRetryMeta {
  attempt?: unknown;
  max_retries?: unknown;
  delay_ms?: unknown;
  error?: unknown;
}

function toInt(v: unknown, fallback: number): number {
  const n = typeof v === 'number' ? v : typeof v === 'string' ? Number(v) : NaN;
  return Number.isFinite(n) ? Math.trunc(n) : fallback;
}

export const useLlmRetryStore = defineStore('llmRetry', () => {
  const retries = ref<Record<string, LlmRetryState>>({});

  function noteRetry(sessionId: string, meta: LlmRetryMeta) {
    const sid = sessionId.trim();
    if (!sid) return;
    retries.value[sid] = {
      attempt: Math.max(1, toInt(meta.attempt, 1)),
      maxRetries: toInt(meta.max_retries, -1),
      delayMs: Math.max(0, toInt(meta.delay_ms, 0)),
      error: typeof meta.error === 'string' ? meta.error : '',
      updatedAt: Date.now(),
    };
  }

  function clear(sessionId: string) {
    if (!sessionId) return;
    delete retries.value[sessionId];
  }

  function clearAll() {
    retries.value = {};
  }

  function retryFor(sessionId: string): LlmRetryState | null {
    if (!sessionId) return null;
    return retries.value[sessionId] ?? null;
  }

  return { retries, noteRetry, clear, clearAll, retryFor };
});
