import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { effectScope, nextTick, ref } from 'vue';
import { flushPromises } from '@vue/test-utils';
import { useChatActivityStore } from '../../../../stores/chat/activityV2Store';
import { useUiConfigStore } from '../../../../stores/uiConfig';
import { useTaskWorkspaceProcess } from '../useTaskWorkspaceProcess';
import type { MemberSession, Step, Task, Turn } from '../../v2Types';

const scopes: ReturnType<typeof effectScope>[] = [];
const task = (id: string, seq = 1, status: Task['Status'] = 'completed', sid = 'A') =>
  ({ ID: id, SessionID: sid, Seq: seq, Status: status, Version: 1, UserMessage: id }) as Task;
const turn = (id: string, taskId: string, seq = 1, sid = 'A', spirit = 'A') =>
  ({
    ID: id,
    TaskID: taskId,
    SessionID: sid,
    SpiritSessionID: spirit,
    Seq: seq,
    Version: 1,
    Status: 'completed',
    AgentKey: 'same-name',
    TeamStageID: '',
  }) as Turn;
const step = (id: string, turnId: string, taskId: string, sid = 'A') =>
  ({
    ID: id,
    TurnID: turnId,
    TaskID: taskId,
    SessionID: sid,
    SpiritSessionID: 'A',
    Kind: 'reply',
    Content: 'Visible reply',
    Status: 'completed',
    Seq: 1,
    Version: 1,
  }) as Step;
function setup(enabled = true) {
  const store = useChatActivityStore();
  const active = ref(enabled),
    sid = ref('A'),
    taskId = ref<string | null>(null),
    turnId = ref<string | null>(null);
  const scope = effectScope();
  scopes.push(scope);
  const api = scope.run(() =>
    useTaskWorkspaceProcess({ enabled: () => active.value, sessionId: () => sid.value, taskId, turnId }),
  )!;
  return { store, api, active, sid, taskId, turnId, scope };
}
beforeEach(() => setActivePinia(createPinia()));
afterEach(() => {
  scopes.splice(0).forEach((scope) => scope.stop());
  vi.restoreAllMocks();
});

describe('task and turn reading selection', () => {
  it('follows the latest active task, and keeps an explicit historical task and turn when new facts arrive', async () => {
    const store = useChatActivityStore();
    store.upsertTask(task('old', 1));
    store.upsertTask(task('active', 2, 'running'));
    store.upsertTurn(turn('old-turn', 'old'));
    store.upsertTurn(turn('active-turn', 'active'));
    for (const id of ['old', 'active']) store.hydratedTaskIds.add(id);
    const c = setup();
    await flushPromises();
    expect(c.api.view.value.task?.ID).toBe('active');
    c.api.selectTask('old');
    await nextTick();
    expect(c.turnId.value).toBe('old-turn');
    store.upsertTask(task('new', 3, 'running'));
    store.hydratedTaskIds.add('new');
    store.upsertTurn(turn('later-turn', 'old', 2));
    await nextTick();
    expect(c.api.view.value.task?.ID).toBe('old');
    expect(c.api.view.value.turn?.ID).toBe('old-turn');
    c.api.followCurrent();
    await nextTick();
    expect(c.api.view.value.task?.ID).toBe('new');
    expect(c.api.view.value.manual).toBe(false);
  });
  it('pins a task when a specific turn is chosen and does not accept a turn from another task', async () => {
    const store = useChatActivityStore();
    store.upsertTask(task('T'));
    store.hydratedTaskIds.add('T');
    store.upsertTurn(turn('one', 'T'));
    store.upsertTurn(turn('two', 'T', 2));
    const c = setup();
    c.api.selectTurn('one');
    await nextTick();
    store.upsertTask(task('new', 2, 'running'));
    c.api.selectTurn('foreign');
    expect([c.taskId.value, c.turnId.value]).toEqual(['T', 'one']);
    expect(c.api.view.value.turn?.ID).toBe('one');
  });
  it('does not fetch hidden process panels and preserves errors until an explicit retry', async () => {
    const store = useChatActivityStore();
    store.upsertTask(task('T'));
    const load = vi.spyOn(store, 'hydrateTask').mockResolvedValue(undefined);
    const c = setup(false);
    await flushPromises();
    expect(load).not.toHaveBeenCalled();
    c.active.value = true;
    await flushPromises();
    expect(c.api.view.value.state).toBe('error');
    await nextTick();
    expect(load).toHaveBeenCalledTimes(1);
    load.mockImplementation(async (id) => {
      store.hydratedTaskIds.add(id);
    });
    c.api.retry();
    await flushPromises();
    expect(c.api.view.value.state).toBe('ready');
  });
  it('ignores stale loading errors during A to B to A and on disposal', async () => {
    const store = useChatActivityStore();
    store.upsertTask(task('TA'));
    store.upsertTask(task('TB', 1, 'running', 'B'));
    let reject!: (error: Error) => void;
    vi.spyOn(store, 'hydrateTask')
      .mockImplementationOnce(
        () =>
          new Promise((_resolve, r) => {
            reject = r;
          }),
      )
      .mockResolvedValue(undefined);
    const c = setup();
    await nextTick();
    c.sid.value = 'B';
    await flushPromises();
    c.sid.value = 'A';
    store.hydratedTaskIds.add('TA');
    await flushPromises();
    reject(new Error('old'));
    await flushPromises();
    expect(c.api.view.value.task?.ID).toBe('TA');
    expect(c.api.view.value.state).toBe('ready');
    c.scope.stop();
  });
  it('checks full task/session membership for turns, steps and exact member details', async () => {
    const store = useChatActivityStore();
    store.upsertTask(task('T'));
    store.hydratedTaskIds.add('T');
    store.upsertTurn(turn('foreign', 'T', 3, 'B', 'B'));
    store.upsertTurn(turn('member-turn', 'T', 2, 'child', 'A'));
    store.upsertStep(step('good', 'member-turn', 'T', 'child'));
    store.upsertStep(step('wrong', 'member-turn', 'other', 'child'));
    store.upsertMemberSession({
      ID: 'wrong-member',
      TaskID: 'other',
      SessionID: 'child',
      SpiritSessionID: 'A',
      Seq: 1,
      Version: 1,
      AgentKey: 'same-name',
    } as MemberSession);
    store.upsertMemberSession({
      ID: 'member',
      TaskID: 'T',
      SessionID: 'child',
      SpiritSessionID: 'A',
      Seq: 1,
      Version: 1,
      AgentKey: 'same-name',
    } as MemberSession);
    const c = setup();
    await flushPromises();
    expect(c.api.view.value.turns.map((row) => row.ID)).toEqual(['member-turn']);
    expect(c.api.view.value.steps.map((row) => row.ID)).toEqual(['good']);
    expect(c.api.view.value.member?.ID).toBe('member');
    expect(c.api.view.value.canLocateTurn).toBe(false);
  });
  it('keeps missing manual targets explicit instead of silently replacing them', async () => {
    const store = useChatActivityStore();
    store.upsertTask(task('T'));
    store.hydratedTaskIds.add('T');
    const c = setup();
    c.taskId.value = 'missing';
    await nextTick();
    expect(c.api.view.value.task).toBeNull();
    expect(c.api.view.value.missingSelection).toBe(true);
    c.api.followCurrent();
    await nextTick();
    expect(c.api.view.value.task?.ID).toBe('T');
  });
  it('binds a step location to its actual turn and filters internal notices', async () => {
    const store = useChatActivityStore();
    store.upsertTask(task('T'));
    store.hydratedTaskIds.add('T');
    store.upsertTurn(turn('one', 'T'));
    store.upsertTurn(turn('two', 'T', 2));
    store.upsertStep(step('s', 'one', 'T'));
    store.upsertStep({ ...step('internal', 'one', 'T'), Kind: 'notice', NoticeType: 'context_usage' });
    const c = setup();
    c.api.selectLocation('T', 's');
    await nextTick();
    expect(c.turnId.value).toBe('one');
    expect(c.api.view.value.steps.map((row) => row.ID)).toEqual(['s']);
  });
  it('uses the main timeline visibility policy for empty replies and hidden tools', async () => {
    const store = useChatActivityStore();
    store.upsertTask(task('T'));
    store.hydratedTaskIds.add('T');
    store.upsertTurn(turn('one', 'T'));
    store.upsertStep({ ...step('empty', 'one', 'T'), Content: '' });
    store.upsertStep({ ...step('tool', 'one', 'T'), Kind: 'action', ToolName: 'do_work' });
    useUiConfigStore().showToolCalls = false;
    const c = setup();
    await flushPromises();
    expect(c.api.view.value.steps).toEqual([]);
    useUiConfigStore().showToolCalls = true;
    expect(c.api.view.value.steps.map((row) => row.ID)).toEqual(['tool']);
  });
});
