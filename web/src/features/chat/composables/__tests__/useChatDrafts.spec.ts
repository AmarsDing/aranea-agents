import { describe, expect, it } from 'vitest';
import { effectScope, ref } from 'vue';
import { useChatDrafts } from '../useChatDrafts';

function setup(initial: string | null = 'A') {
  const input = ref('');
  const sid = ref<string | undefined>(initial ?? undefined);
  const entity = ref('agent-1');
  const scope = effectScope();
  const drafts = scope.run(() =>
    useChatDrafts(
      input,
      () => entity.value,
      () => sid.value,
    ),
  )!;
  return { input, sid, entity, drafts, stop: () => scope.stop() };
}

describe('revision scoped text drafts', () => {
  it('a late A receipt consumes A without clearing B', () => {
    const c = setup();
    c.input.value = 'send A';
    const consume = c.drafts.capture();
    c.sid.value = 'B';
    c.input.value = 'draft B';
    consume();
    expect(c.input.value).toBe('draft B');
    c.sid.value = 'A';
    expect(c.input.value).toBe('');
    c.stop();
  });
  it('retains a new revision even when the user edits back to the same text', () => {
    const c = setup();
    c.input.value = 'same';
    const consume = c.drafts.capture();
    c.input.value = 'changed';
    c.input.value = 'same';
    consume();
    expect(c.input.value).toBe('same');
    c.stop();
  });
  it('isolates unsaved drafts by entity', () => {
    const c = setup(null);
    c.input.value = 'first agent';
    c.entity.value = 'agent-2';
    expect(c.input.value).toBe('');
    c.input.value = 'second agent';
    c.entity.value = 'agent-1';
    expect(c.input.value).toBe('first agent');
    c.stop();
  });
  it('migrates a new session draft using the returned ID without overwriting later input', async () => {
    const c = setup(null);
    c.input.value = 'original instruction';
    const consume = c.drafts.capture();
    await c.drafts.createSession(async () => {
      c.sid.value = 'created';
      c.input.value = 'new text while creating';
      return 'created';
    });
    consume();
    expect(c.input.value).toBe('new text while creating');
    c.stop();
  });
  it('keeps the originating draft under the created ID while viewing another session', async () => {
    const c = setup(null);
    c.input.value = 'original instruction';
    await c.drafts.createSession(async () => {
      c.sid.value = 'B';
      c.input.value = 'draft B';
      return 'created-A';
    });
    expect(c.input.value).toBe('draft B');
    c.sid.value = 'created-A';
    expect(c.input.value).toBe('original instruction');
    c.stop();
  });
});
