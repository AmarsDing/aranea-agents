import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils';
import { defineComponent, ref } from 'vue';
import TaskList from '../TaskList.vue';
import { useScrollToActivity } from '../../../../features/chat/composables/useScrollToActivity';
const fixture = vi.hoisted(() => ({ expandTask: vi.fn().mockResolvedValue(undefined) }));
vi.mock('../../../../features/chat/composables/useActivityQueries', () => ({
  useActivityQueries: () => ({
    getSessionTasks: () => [{ ID: 'T' }],
    isTaskHydrated: () => true,
    taskHydrationState: () => undefined,
  }),
}));
vi.mock('../../../../features/chat/composables/useLazyTaskHydration', () => ({
  CHAT_SCROLL_EL_KEY: 'chat-messages-scroll-el',
  useLazyTaskHydration: () => ({
    expandTask: fixture.expandTask,
    isCollapsed: () => false,
    syncCards: () => undefined,
  }),
}));
const card = defineComponent({
  template: '<div data-task-id="T" tabindex="-1"><div data-turn-id="turn" tabindex="-1">Original turn</div></div>',
});
const wrappers: VueWrapper[] = [];
const roots: HTMLElement[] = [];
afterEach(() => {
  wrappers.splice(0).forEach((w) => w.unmount());
  roots.splice(0).forEach((r) => r.remove());
  useScrollToActivity().taskLocateCommand.value = null;
  fixture.expandTask.mockReset().mockResolvedValue(undefined);
});
function setup() {
  const root = document.createElement('div');
  document.body.appendChild(root);
  roots.push(root);
  const wrapper = mount(TaskList, {
    attachTo: root,
    props: { sessionId: 'A' },
    global: { provide: { 'chat-messages-scroll-el': ref(root) }, stubs: { TaskCard: card } },
  });
  wrappers.push(wrapper);
  const target = root.querySelector<HTMLElement>('[data-turn-id]')!;
  target.scrollIntoView = vi.fn();
  return { wrapper, target };
}
describe('original turn navigation', () => {
  it('expands the task and focuses the exact turn in the original timeline', async () => {
    const c = setup();
    useScrollToActivity().locateTurn('A', 'T', 'turn');
    await flushPromises();
    expect(fixture.expandTask).toHaveBeenCalledWith('T');
    expect(document.activeElement).toBe(c.target);
    expect(c.target.scrollIntoView).toHaveBeenCalledWith({ block: 'start', behavior: 'auto' });
  });
  it('does not focus an old turn after leaving its chat during hydration', async () => {
    let resolve!: () => void;
    fixture.expandTask.mockImplementationOnce(
      () =>
        new Promise<void>((r) => {
          resolve = r;
        }),
    );
    const c = setup();
    useScrollToActivity().locateTurn('A', 'T', 'turn');
    await c.wrapper.vm.$nextTick();
    await c.wrapper.setProps({ sessionId: 'B' });
    resolve();
    await flushPromises();
    expect(c.target.scrollIntoView).not.toHaveBeenCalled();
  });
  it('preserves an existing keyboard tab stop when focusing a record', async () => {
    const c = setup();
    c.target.tabIndex = 0;
    useScrollToActivity().locateTurn('A', 'T', 'turn');
    await flushPromises();
    expect(c.target.tabIndex).toBe(0);
    expect(document.activeElement).toBe(c.target);
  });
});
