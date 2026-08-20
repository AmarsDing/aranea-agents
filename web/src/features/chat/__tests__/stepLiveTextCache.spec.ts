import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  noteStepLiveText,
  readStepLiveText,
  clearStepLiveText,
  mergeStepLiveText,
  isTerminalStepStatus,
} from '../stepLiveTextCache';

describe('stepLiveTextCache', () => {
  const sid = 'turn-1-s1';

  afterEach(() => {
    clearStepLiveText(sid);
    clearStepLiveText('turn-1-s2');
    vi.useRealTimers();
  });

  it('notes and reads accumulated live text', () => {
    noteStepLiveText(sid, 'content', 'Hello');
    noteStepLiveText(sid, 'content', 'Hello world');
    noteStepLiveText(sid, 'reasoning', 'thinking…');
    const got = readStepLiveText(sid);
    expect(got?.content).toBe('Hello world');
    expect(got?.reasoning).toBe('thinking…');
  });

  it('flushes to sessionStorage after the throttle interval (cold-memory hydrate)', () => {
    vi.useFakeTimers();
    noteStepLiveText(sid, 'content', 'persisted text');
    vi.advanceTimersByTime(600);
    expect(sessionStorage.getItem(`aranea:step-live:${sid}`)).toContain('persisted text');
  });

  it('clearStepLiveText removes memory, timer and storage', () => {
    vi.useFakeTimers();
    noteStepLiveText(sid, 'content', 'x');
    clearStepLiveText(sid);
    vi.advanceTimersByTime(1000);
    expect(readStepLiveText(sid)).toBeUndefined();
    expect(sessionStorage.getItem(`aranea:step-live:${sid}`)).toBeNull();
  });

  it('mergeStepLiveText takes the longer prefix-consistent value for non-terminal steps', () => {
    noteStepLiveText(sid, 'content', 'Hello world');
    const merged = mergeStepLiveText({ ID: sid, Content: 'Hello', Reasoning: '', Status: 'running' });
    expect(merged.Content).toBe('Hello world');
    // DB snapshot newer than cache → keep DB (shorter cache does not truncate).
    const kept = mergeStepLiveText({ ID: sid, Content: 'Hello world, again', Reasoning: '', Status: 'running' });
    expect(kept.Content).toBe('Hello world, again');
  });

  it('mergeStepLiveText is a no-op for terminal steps and never truncates on divergence', () => {
    noteStepLiveText(sid, 'content', 'cached tail');
    const terminal = mergeStepLiveText({ ID: sid, Content: 'final', Reasoning: '', Status: 'completed' });
    expect(terminal.Content).toBe('final');
    // 前缀分叉（理论不可能，防御分支）→ 信服务端。
    const diverged = mergeStepLiveText({ ID: sid, Content: 'different stream', Reasoning: '', Status: 'running' });
    expect(diverged.Content).toBe('different stream');
  });

  it('isTerminalStepStatus covers all terminal phases', () => {
    for (const s of ['completed', 'failed', 'cancelled', 'interrupted', 'skipped']) {
      expect(isTerminalStepStatus(s)).toBe(true);
    }
    for (const s of ['running', 'pending', 'created', 'tool_running']) {
      expect(isTerminalStepStatus(s)).toBe(false);
    }
  });

  it('expires orphan entries beyond TTL', () => {
    vi.useFakeTimers();
    noteStepLiveText(sid, 'content', 'old');
    vi.advanceTimersByTime(2 * 60 * 60 * 1000 + 1);
    expect(readStepLiveText(sid)).toBeUndefined();
  });
});
