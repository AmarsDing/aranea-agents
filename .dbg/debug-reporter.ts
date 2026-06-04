/**
 * Debug-only network reporter for the `web-chat-no-response` session.
 * This file is created by the TRAE-debugger instrumentation phase and is
 * removed during cleanup. Do NOT import it from production code paths
 * outside of the current debugging session.
 */
const DEBUG_URL = 'http://127.0.0.1:7777/event';
const DEBUG_SESSION = 'web-chat-no-response';

export type DebugEvent = {
  hypothesisId: string;
  source: string;
  data?: Record<string, unknown>;
  level?: 'info' | 'warn' | 'error';
};

export function reportDebug(ev: DebugEvent): void {
  try {
    void fetch(DEBUG_URL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        sessionId: DEBUG_SESSION,
        hypothesisId: ev.hypothesisId,
        source: ev.source,
        data: ev.data ?? {},
        level: ev.level ?? 'info',
        ts: Date.now(),
      }),
      keepalive: true,
    }).catch(() => {
      /* swallow: instrumentation must never break app */
    });
  } catch {
    /* swallow */
  }
}
