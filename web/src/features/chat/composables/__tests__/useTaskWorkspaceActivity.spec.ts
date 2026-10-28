import { beforeEach, describe, expect, it } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { ref } from 'vue';
import { useTaskWorkspaceActivity } from '../useTaskWorkspaceActivity';
import { useChatActivityStore } from '../../../../stores/chat/activityV2Store';
import type { Task, Step } from '../../v2Types';
const task = (id: string, session = 'A', status = 'running') =>
  ({ ID: id, SessionID: session, Status: status, Seq: 1, Version: 1, UserMessage: id }) as Task;
const step = (id: string, kind = 'confirm', status = 'running', taskId = 'T', session = 'A') =>
  ({
    ID: id,
    TaskID: taskId,
    TurnID: 'turn',
    SessionID: session,
    SpiritSessionID: session,
    Kind: kind,
    Status: status,
    Seq: 1,
    Version: 1,
  }) as Step;
beforeEach(() => setActivePinia(createPinia()));
describe('loaded task and attention projection', () => {
  it('shows only actionable current-session items from active tasks', () => {
    const store = useChatActivityStore();
    store.upsertTask(task('T'));
    store.upsertTask(task('old', 'A', 'completed'));
    for (const row of [
      step('confirm'),
      step('clarify', 'clarify', 'awaiting_input'),
      step('done', 'confirm', 'completed'),
      step('foreign', 'confirm', 'running', 'T', 'B'),
      step('stale', 'confirm', 'running', 'old'),
    ])
      store.upsertStep(row);
    const view = useTaskWorkspaceActivity(() => 'A');
    expect(view.pendingSteps.value.map((row) => row.ID)).toEqual(['confirm', 'clarify']);
    expect(view.incomplete.value).toBe(true);
    expect(store.getSessionTasks('A')).toHaveLength(2);
  });
  it('updates after the original card completes without a second action surface', () => {
    const store = useChatActivityStore();
    store.upsertTask(task('T'));
    store.upsertStep(step('confirm'));
    const view = useTaskWorkspaceActivity(() => 'A');
    expect(view.pendingSteps.value).toHaveLength(1);
    store.upsertStep({ ...step('confirm'), Status: 'completed', Version: 2 });
    expect(view.pendingSteps.value).toHaveLength(0);
  });
  it('switches view scope without changing tasks or carrying pending items into B', () => {
    const store = useChatActivityStore();
    store.upsertTask(task('T'));
    store.upsertStep(step('confirm'));
    const sid = ref('A');
    const view = useTaskWorkspaceActivity(() => sid.value);
    sid.value = 'B';
    expect(view.tasks.value).toHaveLength(0);
    expect(view.pendingSteps.value).toHaveLength(0);
    expect(store.tasks.size).toBe(1);
  });
});
