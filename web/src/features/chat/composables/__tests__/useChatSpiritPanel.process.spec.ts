import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { effectScope, nextTick, reactive } from 'vue';
import { useChatSpiritPanel } from '../useChatSpiritPanel';
import { useChatActivityStore } from '../../../../stores/chat/activityV2Store';
import type { MemberSession, Task } from '../../v2Types';
vi.mock('../../../../stores/spirit', () => ({ useSpiritTeamStore: () => ({ teams: [], activeTeamId: null }) }));
vi.mock('../../../../stores/chat/runtimeStore', () => ({ useChatRuntimeStore: () => ({}) }));
vi.mock('../useChatSender', () => ({ USER_INPUT_HARD_LIMIT_CHARS: 10000 }));
const scopes: ReturnType<typeof effectScope>[] = [];
beforeEach(() => setActivePinia(createPinia()));
afterEach(() => scopes.splice(0).forEach((s) => s.stop()));
describe('exact process member detail navigation', () => {
  it('rejects a different task or parent and closes details when the viewed chat changes', async () => {
    const store = useChatActivityStore();
    store.upsertTask({ ID: 'T', SessionID: 'A', Seq: 1, Version: 1 } as Task);
    store.upsertMemberSession({
      ID: 'member',
      TaskID: 'T',
      SessionID: 'child',
      SpiritSessionID: 'A',
      Seq: 1,
      Version: 1,
    } as MemberSession);
    store.upsertMemberSession({
      ID: 'wrong',
      TaskID: 'other',
      SessionID: 'child',
      SpiritSessionID: 'B',
      Seq: 1,
      Version: 1,
    } as MemberSession);
    store.upsertMemberSession({
      ID: 'wrong-task',
      TaskID: 'other',
      SessionID: 'child',
      SpiritSessionID: 'A',
      Seq: 1,
      Version: 1,
    } as MemberSession);
    store.upsertMemberSession({
      ID: 'wrong-parent',
      TaskID: 'T',
      SessionID: 'child',
      SpiritSessionID: 'B',
      Seq: 1,
      Version: 1,
    } as MemberSession);
    const session = reactive({ selectedSessionForUi: { id: 'A' } });
    const scope = effectScope();
    scopes.push(scope);
    const api = scope.run(() => useChatSpiritPanel({ session, entity: {}, layout: {} } as never))!;
    api.openTaskMember('T', 'wrong');
    expect(api.sidebarMemberDialogOpen.value).toBe(false);
    api.openTaskMember('T', 'wrong-task');
    expect(api.sidebarMemberDialogOpen.value).toBe(false);
    api.openTaskMember('T', 'wrong-parent');
    expect(api.sidebarMemberDialogOpen.value).toBe(false);
    api.openTaskMember('T', 'member');
    expect(api.sidebarActiveMember.value?.ID).toBe('member');
    expect(api.sidebarMemberDialogOpen.value).toBe(true);
    expect(session.selectedSessionForUi.id).toBe('A');
    session.selectedSessionForUi = { id: 'B' };
    expect(api.sidebarActiveMember.value).toBeNull();
    await nextTick();
    expect(api.sidebarMemberDialogOpen.value).toBe(false);
  });
});
