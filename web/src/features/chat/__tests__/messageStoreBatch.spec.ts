import { describe, expect, it, vi } from 'vitest';
import { createMessageBatchWriter } from '../messageStoreBatch';
import type { Message } from '../types';

function msg(id: string, content = ''): Message {
  return {
    id,
    session_id: 's1',
    parent_message_id: '',
    turn_id: '',
    turn_number: 0,
    seq_in_turn: 0,
    role: 'assistant',
    content_markdown: content,
    model_name: 'gpt-4',
    token_in: 0,
    token_out: 0,
    latency_ms: 0,
    status: 'streaming',
    attachments_count: 0,
    options_json: '',
    error_message: '',
    created_at: new Date().toISOString(),
    origin: { kind: 'streaming', sessionId: 's1' },
    agent_ref: null,
    team_member: null,
    source_meta: null,
  };
}

describe('createMessageBatchWriter', () => {
  it('inserts messages in the next animation frame', async () => {
    let rows: Message[] = [msg('a')];
    const writer = createMessageBatchWriter(
      () => rows,
      (next) => {
        rows = next;
      },
    );

    writer.insert(msg('b'));
    expect(rows).toHaveLength(1);

    await new Promise((resolve) => requestAnimationFrame(resolve));
    expect(rows).toHaveLength(2);
    expect(rows[1].id).toBe('b');

    writer.dispose();
  });

  it('patches existing messages by id without multiple array copies', async () => {
    let rows: Message[] = [msg('a', 'hello'), msg('b', 'world')];
    const setMessages = vi.fn((next: Message[]) => {
      rows = next;
    });
    const writer = createMessageBatchWriter(() => rows, setMessages);

    writer.update('a', (m) => ({ ...m, content_markdown: m.content_markdown + '!' }));
    writer.update('a', (m) => ({ ...m, content_markdown: m.content_markdown + '?' }));
    writer.update('b', (m) => ({ ...m, content_markdown: m.content_markdown + '.' }));

    expect(setMessages).not.toHaveBeenCalled();

    writer.flushSync();
    expect(rows[0].content_markdown).toBe('hello!?');
    expect(rows[1].content_markdown).toBe('world.');
    expect(setMessages).toHaveBeenCalledTimes(1);

    writer.dispose();
  });

  it('can patch an inserted message before flush', async () => {
    let rows: Message[] = [];
    const writer = createMessageBatchWriter(
      () => rows,
      (next) => {
        rows = next;
      },
    );

    writer.insert(msg('x', 'A'));
    writer.update('x', (m) => ({ ...m, content_markdown: 'B' }));

    writer.flushSync();
    expect(rows).toHaveLength(1);
    expect(rows[0].content_markdown).toBe('B');

    writer.dispose();
  });

  it('drops pending work after dispose', async () => {
    let rows: Message[] = [];
    const writer = createMessageBatchWriter(
      () => rows,
      (next) => {
        rows = next;
      },
    );

    writer.insert(msg('x'));
    writer.dispose();

    await new Promise((resolve) => requestAnimationFrame(resolve));
    expect(rows).toHaveLength(0);
  });
});
