import { afterEach, describe, expect, it, vi } from 'vitest';
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils';
import { defineComponent, ref } from 'vue';
import TaskList from '../TaskList.vue';
import { useScrollToActivity } from '../../../../features/chat/composables/useScrollToActivity';
import { CHAT_SCROLL_EL_KEY } from '../../../../features/chat/composables/useLazyTaskHydration';

const fixture = vi.hoisted(() => ({
  expandTask: vi.fn().mockResolvedValue(undefined),
  syncCards: vi.fn(),
  hydrateTask: vi.fn(),
}));
vi.mock('../../../../features/chat/composables/useActivityQueries', () => ({
  useActivityQueries: () => ({
    getSessionTasks: () => [{ ID: 'task-1' }],
    isTaskHydrated: () => true,
    taskHydrationState: () => 'idle',
    hydrateTask: fixture.hydrateTask,
  }),
}));
vi.mock('../../../../features/chat/composables/useLazyTaskHydration', () => ({
  CHAT_SCROLL_EL_KEY: 'chat-messages-scroll-el',
  useLazyTaskHydration: () => ({ ...fixture, isCollapsed: () => false, toggleCollapse: vi.fn() }),
}));
const card = defineComponent({
  props: { task: { type: Object, required: true } },
  template:
    '<div :data-task-id="task.ID" tabindex="-1"><div data-step-id="step-1" tabindex="-1">Original confirmation</div></div>',
});
const wrappers: VueWrapper[] = [];
const roots: HTMLElement[] = [];
afterEach(() => {
  wrappers.splice(0).forEach((w) => w.unmount());
  roots.splice(0).forEach((r) => r.remove());
  useScrollToActivity().taskLocateCommand.value = null;
  vi.restoreAllMocks();
  fixture.expandTask.mockReset().mockResolvedValue(undefined);
});
function setup() {
  const root = document.createElement('div');
  document.body.appendChild(root);
  roots.push(root);
  const wrapper = mount(TaskList, {
    props: { sessionId: 'A' },
    attachTo: root,
    global: { provide: { [CHAT_SCROLL_EL_KEY]: ref(root) }, stubs: { TaskCard: card } },
  });
  wrappers.push(wrapper);
  const step = root.querySelector<HTMLElement>('[data-step-id]')!;
  step.scrollIntoView = vi.fn();
  return { wrapper, step };
}
describe('task attention navigation', () => {
  it('awaits expansion and focuses the original step without duplicating actions', async () => {
    const c = setup();
    useScrollToActivity().locateTask('A', 'task-1', 'step-1');
    await flushPromises();
    expect(fixture.expandTask).toHaveBeenCalledWith('task-1');
    expect(document.activeElement).toBe(c.step);
    expect(c.step.scrollIntoView).toHaveBeenCalledWith({ block: 'start', behavior: 'auto' });
  });
  it('discards an in-flight locate when the visible session changes', async () => {
    let resolve!: () => void;
    fixture.expandTask.mockImplementationOnce(
      () =>
        new Promise<void>((r) => {
          resolve = r;
        }),
    );
    const c = setup();
    useScrollToActivity().locateTask('A', 'task-1', 'step-1');
    await c.wrapper.vm.$nextTick();
    await c.wrapper.setProps({ sessionId: 'B' });
    resolve();
    await flushPromises();
    expect(c.step.scrollIntoView).not.toHaveBeenCalled();
    expect(document.activeElement).not.toBe(c.step);
  });
});
