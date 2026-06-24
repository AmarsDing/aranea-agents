import { describe, expect, it } from 'vitest';
import type { Message } from '../types';
import {
  mergeIncrementalSessionMessages,
  mergeSessionMessages,
  dropPendingUserPlaceholders,
} from '../mergeSessionMessages';

function msg(id: string, status = 'ok', created = '2026-05-20T10:00:00Z'): Message {
  return {
    id,
    session_id: 'sess-1',
    parent_message_id: '',
    turn_id: '',
    turn_number: 0,
    seq_in_turn: 0,
    role: 'assistant',
    content_markdown: '',
    model_name: '',
    token_in: 0,
    token_out: 0,
    latency_ms: 0,
    status,
    attachments_count: 0,
    options_json: '',
    error_message: '',
    created_at: created,
  };
}

describe('mergeSessionMessages', () => {
  it('keeps streaming row while merging server history', () => {
    const server = [msg('u-1', 'ok', '2026-05-20T10:00:00Z')];
    const local = [
      ...server,
      msg('ws-stream-sess-1', 'streaming', '2026-05-20T10:00:01Z'),
      msg('act-tc-1', 'tool_running', '2026-05-20T10:00:02Z'),
    ];
    const merged = mergeSessionMessages(server, local);
    expect(merged.some((m) => m.id === 'ws-stream-sess-1')).toBe(true);
    expect(merged.some((m) => m.id === 'act-tc-1')).toBe(true);
  });

  it('sorts by time with in-flight rows after persisted', () => {
    const server = [{ ...msg('a', 'ok', '2026-05-20T10:00:02Z') }, { ...msg('b', 'ok', '2026-05-20T10:00:01Z') }];
    const local = [{ ...msg('ws-stream-sess-1', 'streaming', '2026-05-20T10:00:03Z') }];
    const merged = mergeSessionMessages(server, local);
    // Persisted sorted by created_at, in-flight after
    expect(merged.map((m) => m.id)).toEqual(['b', 'a', 'ws-stream-sess-1']);
  });

  it('drops pending-user placeholders', () => {
    const rows = [{ ...msg('pending-user-1'), role: 'user', content_markdown: 'hi' }, msg('u-1')];
    const out = dropPendingUserPlaceholders(rows);
    expect(out.some((m) => m.id === 'pending-user-1')).toBe(false);
    expect(out.some((m) => m.id === 'u-1')).toBe(true);
  });

  it('drops stale in-flight rows when dropStaleInFlight is set', () => {
    const server = [msg('u-1', 'ok', '2026-05-20T10:00:00Z')];
    const local = [...server, msg('act-tc-1', 'tool_running', '2026-05-20T10:00:02Z')];
    const merged = mergeSessionMessages(server, local, { dropStaleInFlight: true });
    expect(merged.some((m) => m.id === 'act-tc-1')).toBe(false);
  });

  it('keeps failed pending-user rows when dropStaleInFlight is set', () => {
    const server = [msg('u-1', 'ok', '2026-05-20T10:00:00Z')];
    const failed = { ...msg('pending-user-1', 'failed', '2026-05-20T10:00:02Z'), role: 'user' };
    const merged = mergeSessionMessages(server, [...server, failed], { dropStaleInFlight: true });
    expect(merged.some((m) => m.id === 'pending-user-1')).toBe(true);
  });

  it('keeps streaming ws-stream row when dropStaleInFlight is set', () => {
    const server = [msg('u-1', 'ok', '2026-05-20T10:00:00Z')];
    const local = [...server, msg('ws-stream-sess-1', 'streaming', '2026-05-20T10:00:01Z')];
    const merged = mergeSessionMessages(server, local, { dropStaleInFlight: true });
    expect(merged.some((m) => m.id === 'ws-stream-sess-1')).toBe(true);
  });

  it('drops terminal ws-stream row when dropStaleInFlight is set', () => {
    const server = [msg('u-1', 'ok', '2026-05-20T10:00:00Z'), msg('asst-1', 'ok', '2026-05-20T10:00:02Z')];
    const local = [
      ...server,
      { ...msg('ws-stream-sess-1', 'ok', '2026-05-20T10:00:02Z'), content_markdown: 'duplicate answer' },
    ];
    const merged = mergeSessionMessages(server, local, { dropStaleInFlight: true });
    expect(merged.some((m) => m.id === 'ws-stream-sess-1')).toBe(false);
    expect(merged.some((m) => m.id === 'asst-1')).toBe(true);
  });

  it('mergeIncrementalSessionMessages retains persisted history (M55-SYNC / DECO-01)', () => {
    const existing = [
      { ...msg('u-1', 'ok', '2026-05-20T10:00:00Z'), role: 'user', content_markdown: 'hello' },
      { ...msg('a-1', 'ok', '2026-05-20T10:00:01Z'), role: 'assistant', content_markdown: 'hi' },
    ];
    const local = [...existing, msg('ws-stream-sess-1', 'streaming', '2026-05-20T10:00:02Z')];
    const incremental = [
      { ...msg('u-2', 'ok', '2026-05-20T10:00:03Z'), role: 'user', content_markdown: 'from feishu' },
    ];
    const merged = mergeIncrementalSessionMessages(incremental, local);
    expect(merged.map((m) => m.id)).toContain('u-1');
    expect(merged.map((m) => m.id)).toContain('a-1');
    expect(merged.map((m) => m.id)).toContain('u-2');
    expect(merged.map((m) => m.id)).toContain('ws-stream-sess-1');
    expect(merged).toHaveLength(4);
  });

  it('replaces pending-user placeholder with server-persisted user message on merge', () => {
    const serverUser = { ...msg('srv-u-1', 'ok', '2026-05-20T10:00:00Z'), role: 'user', content_markdown: 'hello' };
    const server = [serverUser];
    const local = [
      { ...msg('pending-user-abc', 'ok', '2026-05-20T10:00:00Z'), role: 'user', content_markdown: 'hello' },
    ];
    const merged = mergeSessionMessages(server, local);
    // Server message replaces the placeholder — no duplicate
    expect(merged.some((m) => m.id === 'srv-u-1')).toBe(true);
    expect(merged.some((m) => m.id === 'pending-user-abc')).toBe(false);
    expect(merged).toHaveLength(1);
  });

  it('keeps pending-user placeholder when no matching server message exists', () => {
    const server = [msg('asst-1', 'ok', '2026-05-20T10:00:01Z')];
    const local = [
      { ...msg('pending-user-abc', 'ok', '2026-05-20T10:00:00Z'), role: 'user', content_markdown: 'hello' },
    ];
    const merged = mergeSessionMessages(server, local);
    expect(merged.some((m) => m.id === 'pending-user-abc')).toBe(true);
    expect(merged).toHaveLength(2);
  });

  it('keeps failed pending-user placeholder even when matching server message exists', () => {
    const serverUser = { ...msg('srv-u-1', 'ok', '2026-05-20T10:00:00Z'), role: 'user', content_markdown: 'hello' };
    const server = [serverUser];
    const local = [
      { ...msg('pending-user-abc', 'failed', '2026-05-20T10:00:00Z'), role: 'user', content_markdown: 'hello' },
    ];
    const merged = mergeSessionMessages(server, local);
    // Failed placeholder is preserved for retry UX
    expect(merged.some((m) => m.id === 'pending-user-abc')).toBe(true);
    expect(merged.some((m) => m.id === 'srv-u-1')).toBe(true);
  });

  it('replaces pending-user placeholder with dropStaleInFlight', () => {
    const serverUser = { ...msg('srv-u-1', 'ok', '2026-05-20T10:00:00Z'), role: 'user', content_markdown: 'hello' };
    const server = [serverUser];
    const local = [
      { ...msg('pending-user-abc', 'ok', '2026-05-20T10:00:00Z'), role: 'user', content_markdown: 'hello' },
    ];
    const merged = mergeSessionMessages(server, local, { dropStaleInFlight: true });
    expect(merged.some((m) => m.id === 'srv-u-1')).toBe(true);
    expect(merged.some((m) => m.id === 'pending-user-abc')).toBe(false);
  });

  it('keeps terminal local rows with tool_event as persisted', () => {
    const serverUser = { ...msg('srv-u-1', 'ok', '2026-05-20T10:00:00Z'), role: 'user', content_markdown: 'hello' };
    const server = [serverUser];
    const local = [
      serverUser,
      {
        ...msg('act-tc-1', 'ok', '2026-05-20T10:00:01Z'),
        options_json: JSON.stringify({ schema: 'chat.activity/v1', tool_event: { id: 'tc-1' } }),
      },
    ];
    const merged = mergeSessionMessages(server, local);
    expect(merged.some((m) => m.id === 'act-tc-1')).toBe(true);
    expect(merged.findIndex((m) => m.id === 'act-tc-1')).toBeGreaterThan(merged.findIndex((m) => m.id === 'srv-u-1'));
  });
});
