// M74 V2-T4: client tool bridge — desktop executor service tests.
import { describe, it, expect, vi, afterEach } from 'vitest';
import {
  buildClientToolResultFrame,
  createTauriClientToolExecutor,
  executeClientToolInvoke,
  isDesktopCompanion,
  DESKTOP_COMPANION_CAPABILITY,
  type ClientToolExecutor,
  type ClientToolOutcome,
} from '../clientTools';
import type { ClientToolInvokePayload } from '../../realtime/ws-transport';

function invokeMsg(overrides?: Partial<ClientToolInvokePayload>): ClientToolInvokePayload {
  return {
    invocation_id: 'inv-1',
    session_id: 's1',
    tool: 'client_open_app',
    args: { target: 'wechat' },
    ...overrides,
  };
}

function okOutcome(output = 'launched wechat'): ClientToolOutcome {
  return { ok: true, output, error: '', error_code: '' };
}

describe('isDesktopCompanion', () => {
  afterEach(() => {
    delete (window as Record<string, unknown>).__TAURI_INTERNALS__;
  });

  it('is false in a plain browser', () => {
    expect(isDesktopCompanion()).toBe(false);
  });

  it('is true inside the desktop Tauri shell', () => {
    (window as Record<string, unknown>).__TAURI_INTERNALS__ = {};
    expect(isDesktopCompanion()).toBe(true);
  });

  it('is false on Android (UNSUPPORTED_CAPABILITY on the Rust side)', () => {
    (window as Record<string, unknown>).__TAURI_INTERNALS__ = {};
    const original = navigator.userAgent;
    Object.defineProperty(navigator, 'userAgent', {
      value: 'Mozilla/5.0 (Linux; Android 14)',
      configurable: true,
    });
    try {
      expect(isDesktopCompanion()).toBe(false);
    } finally {
      Object.defineProperty(navigator, 'userAgent', { value: original, configurable: true });
    }
  });

  it('exposes the backend capability name', () => {
    expect(DESKTOP_COMPANION_CAPABILITY).toBe('desktop_companion');
  });
});

describe('createTauriClientToolExecutor', () => {
  afterEach(() => {
    delete (window as Record<string, unknown>).__TAURI_INTERNALS__;
  });

  it('returns null outside the desktop shell', () => {
    expect(createTauriClientToolExecutor()).toBeNull();
  });
});

describe('executeClientToolInvoke', () => {
  it('routes client_open_app with its target arg', async () => {
    const exec: ClientToolExecutor = vi.fn(async () => okOutcome());
    const outcome = await executeClientToolInvoke(exec, invokeMsg());
    expect(exec).toHaveBeenCalledWith('client_open_app', { target: 'wechat' });
    expect(outcome.ok).toBe(true);
  });

  it('routes client_open_url with its url arg', async () => {
    const exec: ClientToolExecutor = vi.fn(async () => okOutcome('opened'));
    const outcome = await executeClientToolInvoke(
      exec,
      invokeMsg({ tool: 'client_open_url', args: { url: 'https://example.com' } }),
    );
    expect(exec).toHaveBeenCalledWith('client_open_url', { url: 'https://example.com' });
    expect(outcome.ok).toBe(true);
  });

  it('rejects unknown tools without calling the executor', async () => {
    const exec: ClientToolExecutor = vi.fn(async () => okOutcome());
    const outcome = await executeClientToolInvoke(
      exec,
      invokeMsg({ tool: 'client_screenshot', args: {} }),
    );
    expect(exec).not.toHaveBeenCalled();
    expect(outcome.ok).toBe(false);
    expect(outcome.error_code).toBe('UNSUPPORTED_TOOL');
  });

  it('rejects missing required args without calling the executor', async () => {
    const exec: ClientToolExecutor = vi.fn(async () => okOutcome());
    const outcome = await executeClientToolInvoke(exec, invokeMsg({ args: {} }));
    expect(exec).not.toHaveBeenCalled();
    expect(outcome.ok).toBe(false);
    expect(outcome.error_code).toBe('INVALID_ARGS');
  });

  it('rejects non-string arg values', async () => {
    const exec: ClientToolExecutor = vi.fn(async () => okOutcome());
    const outcome = await executeClientToolInvoke(exec, invokeMsg({ args: { target: 42 } }));
    expect(exec).not.toHaveBeenCalled();
    expect(outcome.error_code).toBe('INVALID_ARGS');
  });

  it('maps executor rejections to EXECUTOR_ERROR', async () => {
    const exec: ClientToolExecutor = async () => {
      throw new Error('ipc broken');
    };
    const outcome = await executeClientToolInvoke(exec, invokeMsg());
    expect(outcome.ok).toBe(false);
    expect(outcome.error_code).toBe('EXECUTOR_ERROR');
    expect(outcome.error).toContain('ipc broken');
  });

  it('passes through Rust-side failures (e.g. NOT_WHITELISTED)', async () => {
    const exec: ClientToolExecutor = async () => ({
      ok: false,
      output: '',
      error: 'target "steam" is not in the client whitelist',
      error_code: 'NOT_WHITELISTED',
    });
    const outcome = await executeClientToolInvoke(exec, invokeMsg({ args: { target: 'steam' } }));
    expect(outcome.ok).toBe(false);
    expect(outcome.error_code).toBe('NOT_WHITELISTED');
  });
});

describe('buildClientToolResultFrame', () => {
  it('builds the success uplink frame', () => {
    const frame = buildClientToolResultFrame(invokeMsg(), okOutcome('launched wechat'));
    expect(frame.direction).toBe('client_to_server');
    expect(frame.channel).toBe('system');
    expect(frame.type).toBe('client_tool.result');
    expect(frame.payload).toEqual({
      invocation_id: 'inv-1',
      ok: true,
      output: 'launched wechat',
      error: '',
    });
  });

  it('prefixes the error_code into the error text on failure', () => {
    const frame = buildClientToolResultFrame(invokeMsg(), {
      ok: false,
      output: '',
      error: 'not whitelisted',
      error_code: 'NOT_WHITELISTED',
    });
    const payload = frame.payload as { invocation_id: string; ok: boolean; error: string };
    expect(payload.invocation_id).toBe('inv-1');
    expect(payload.ok).toBe(false);
    expect(payload.error).toBe('NOT_WHITELISTED: not whitelisted');
  });
});
