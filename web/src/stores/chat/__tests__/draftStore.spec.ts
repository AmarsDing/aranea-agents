import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { effectScope, ref } from 'vue';
import { useChatDraftStore } from '../draftStore';
import { useChatDrafts } from '../../../features/chat/composables/useChatDrafts';
import { CHAT_DRAFT_KEY, clearChatDrafts } from '../../../features/chat/chatDraftPersistence';

const stores: ReturnType<typeof useChatDraftStore>[] = [];
const scopes: ReturnType<typeof effectScope>[] = [];
function page(owner = ref('server/user-1/agent-1'), sid = ref<string | undefined>('A')) {
  const input = ref('');
  const store = useChatDraftStore();
  if (!stores.includes(store)) stores.push(store);
  const scope = effectScope();
  scopes.push(scope);
  const api = scope.run(() =>
    useChatDrafts(
      input,
      () => owner.value,
      () => sid.value,
      { records: store.records, persist: store.persist },
    ),
  )!;
  return { input, owner, sid, api, store, scope };
}
beforeEach(() => {
  sessionStorage.clear();
  setActivePinia(createPinia());
});
afterEach(() => {
  for (const scope of scopes.splice(0)) scope.stop();
  for (const store of stores.splice(0)) store.$dispose();
  vi.restoreAllMocks();
});

describe('identity scoped drafts across page lifetimes', () => {
  it('restores text on route return and after a fresh Pinia instance', () => {
    const a = page();
    a.input.value = 'unfinished changes';
    a.scope.stop();
    const returned = page();
    expect(returned.input.value).toBe('unfinished changes');
    returned.scope.stop();
    setActivePinia(createPinia());
    expect(page().input.value).toBe('unfinished changes');
  });
  it('a late receipt updates a restored page without leaving duplicate text', () => {
    const a = page();
    a.input.value = 'original';
    const consume = a.api.capture();
    a.scope.stop();
    const returned = page();
    consume();
    expect(returned.input.value).toBe('');
    expect(JSON.parse(sessionStorage.getItem(CHAT_DRAFT_KEY)!)).toEqual([]);
  });
  it('a late receipt preserves edits made on the restored page', () => {
    const a = page();
    a.input.value = 'original';
    const consume = a.api.capture();
    a.scope.stop();
    const returned = page();
    returned.input.value = 'new edit';
    consume();
    expect(returned.input.value).toBe('new edit');
  });
  it('separates backend/user/entity identities', () => {
    const a = page();
    a.input.value = 'private';
    a.owner.value = 'server/user-2/agent-1';
    expect(a.input.value).toBe('');
    a.owner.value = 'another-server/user-1/agent-1';
    expect(a.input.value).toBe('');
    a.owner.value = 'server/user-1/agent-1';
    expect(a.input.value).toBe('private');
  });
  it('logout clears persisted/live content and a late session creation cannot resurrect it', async () => {
    const a = page(ref('user-1'), ref(undefined));
    a.input.value = 'private';
    let resolve!: (id: string) => void;
    const creation = a.api.createSession(
      () =>
        new Promise<string>((r) => {
          resolve = r;
        }),
    );
    clearChatDrafts();
    resolve('created');
    await creation;
    expect(a.input.value).toBe('');
    expect(a.store.records.size).toBe(0);
    expect(sessionStorage.getItem(CHAT_DRAFT_KEY)).toBeNull();
  });
  it('reports storage failures while retaining usable in-memory text', () => {
    const a = page();
    vi.spyOn(sessionStorage, 'setItem').mockImplementation(() => {
      throw new Error('quota');
    });
    a.input.value = 'retain this';
    expect(a.store.saveFailed).toBe(true);
    a.scope.stop();
    expect(page().input.value).toBe('retain this');
  });
});
