/**
 * M74 V2-T4: client tool bridge — desktop executor service (design 74 §6.2/§6.3).
 *
 * The backend routes `client_tool.invoke` frames to session WS connections
 * that advertised the desktop_companion capability. This module:
 *   - detects the desktop Tauri shell ({@link isDesktopCompanion}),
 *   - maps invoke payloads onto the Rust commands (client_open_app /
 *     client_open_url) via {@link executeClientToolInvoke},
 *   - builds the `client_tool.result` uplink frame ({@link buildClientToolResultFrame}).
 *
 * Mirrors the localNotification.ts pattern: every entry point degrades to a
 * no-op outside the Tauri shell so callers can wire it unconditionally; the
 * Tauri API is imported lazily so non-Tauri bundles never evaluate it.
 *
 * Security note: the Rust side re-enforces the launch whitelist and URL
 * scheme policy — this layer only forwards validated intent, never raw paths.
 */

import type { ClientToolInvokePayload, WsUpstream } from '../realtime/ws-transport';

/** Capability name registered over WS; must match the backend constant. */
export const DESKTOP_COMPANION_CAPABILITY = 'desktop_companion';

/** Structured outcome returned by the Rust commands (snake_case fields). */
export type ClientToolOutcome = {
  ok: boolean;
  output: string;
  error: string;
  error_code: string;
};

/** Injected command runner; the real one wraps Tauri `invoke`. */
export type ClientToolExecutor = (
  command: string,
  args: Record<string, unknown>,
) => Promise<ClientToolOutcome>;

/** Tools this client can execute, mapped to their Tauri command names. */
const TOOL_COMMANDS: Record<string, { command: string; arg: 'target' | 'url' }> = {
  client_open_app: { command: 'client_open_app', arg: 'target' },
  client_open_url: { command: 'client_open_url', arg: 'url' },
};

function failureOutcome(code: string, message: string): ClientToolOutcome {
  return { ok: false, output: '', error: message, error_code: code };
}

/**
 * True only in the desktop Tauri shell. Android shares `__TAURI_INTERNALS__`
 * but the Rust commands return UNSUPPORTED_CAPABILITY there, so the client
 * must not advertise the capability at all.
 */
export function isDesktopCompanion(): boolean {
  if (typeof window === 'undefined' || !('__TAURI_INTERNALS__' in window)) return false;
  return !/android/i.test(navigator.userAgent);
}

/**
 * Executor backed by Tauri `invoke`; null outside the desktop shell.
 * The API module is imported lazily (cached by the bundler after first use).
 */
export function createTauriClientToolExecutor(): ClientToolExecutor | null {
  if (!isDesktopCompanion()) return null;
  return async (command, args) => {
    const { invoke } = await import('@tauri-apps/api/core');
    return (await invoke(command, args)) as ClientToolOutcome;
  };
}

/**
 * Execute one client_tool.invoke payload: validates tool/args, delegates to
 * the executor, and normalizes every failure into a structured outcome so
 * the caller can always answer the bridge (never leave a pending invocation
 * hanging until timeout when the failure is known locally).
 */
export async function executeClientToolInvoke(
  executor: ClientToolExecutor,
  msg: ClientToolInvokePayload,
): Promise<ClientToolOutcome> {
  const spec = TOOL_COMMANDS[msg.tool];
  if (!spec) {
    return failureOutcome('UNSUPPORTED_TOOL', `tool ${msg.tool} is not supported by this client`);
  }
  const value = msg.args?.[spec.arg];
  if (typeof value !== 'string' || value.trim() === '') {
    return failureOutcome('INVALID_ARGS', `tool ${msg.tool} requires a non-empty string arg "${spec.arg}"`);
  }
  try {
    return await executor(spec.command, { [spec.arg]: value });
  } catch (err) {
    return failureOutcome('EXECUTOR_ERROR', err instanceof Error ? err.message : String(err));
  }
}

/**
 * Build the client_tool.result uplink frame. The backend envelope carries
 * only {invocation_id, ok, output, error}; the machine-readable error_code
 * is prefixed into the error text so the Agent can still paraphrase it.
 */
export function buildClientToolResultFrame(
  msg: ClientToolInvokePayload,
  outcome: ClientToolOutcome,
): WsUpstream {
  return {
    direction: 'client_to_server',
    channel: 'system',
    type: 'client_tool.result',
    payload: {
      invocation_id: msg.invocation_id,
      ok: outcome.ok,
      output: outcome.output,
      error: outcome.ok
        ? ''
        : outcome.error_code
          ? `${outcome.error_code}: ${outcome.error}`
          : outcome.error,
    },
  };
}
