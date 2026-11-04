import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount, type VueWrapper } from '@vue/test-utils';
import { defineComponent, ref } from 'vue';
import TaskList from '../TaskList.vue';
import { CHAT_SCROLL_EL_KEY } from '../../../../features/chat/composables/useLazyTaskHydration';
import type { Task } from '../../../../features/chat/v2Types';

const fixture = vi.hoisted(() => ({
  tasks: [] as Task[],
  expandTask: vi.fn().mockResolvedValue(undefined),
  syncCards: vi.fn(),
}));

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}));
vi.mock('../../../../features/chat/composables/useActivityQueries', () => ({
  useActivityQueries: () => ({
    getSessionTasks: () => fixture.tasks,
    isTaskHydrated: () => true,
    taskHydrationState: () => 'idle',
    hydrateTask: vi.fn(),
  }),
}));
vi.mock('../../../../features/chat/composables/useLazyTaskHydration', () => ({
  CHAT_SCROLL_EL_KEY: 'chat-messages-scroll-el',
  useLazyTaskHydration: () => ({
    expandTask: fixture.expandTask,
    syncCards: fixture.syncCards,
    isCollapsed: () => false,
    toggleCollapse: vi.fn(),
  }),
}));

const card = defineComponent({
  props: { task: { type: Object, required: true } },
  template: '<div class="task-card" :data-task-id="task.ID" :data-task-seq="task.Seq">{{ task.UserMessage }}</div>',
});
const wrappers: VueWrapper[] = [];

function makeTasks(count: number): Task[] {
  return Array.from({ length: count }, (_, index) => ({
    ID: `task-${String(index + 1).padStart(4, '0')}`,
    SessionID: 'S',
    UserMessage: `message-${index + 1}`,
    Status: 'completed',
    Seq: index + 1,
    Version: 1,
    CreatedAt: '2026-11-01T00:00:00Z',
    UpdatedAt: '2026-11-01T00:00:00Z',
  })) as Task[];
}

function setup(count = 1000) {
  fixture.tasks = makeTasks(count);
  const scrollEl = document.createElement('div');
  const wrapper = mount(TaskList, {
    props: { sessionId: 'S' },
    global: { provide: { [CHAT_SCROLL_EL_KEY]: ref(scrollEl) }, stubs: { TaskCard: card } },
  });
  wrappers.push(wrapper);
  return wrapper;
}

function ids(wrapper: VueWrapper) {
  return wrapper.findAll('.task-card[data-task-id]').map((node) => node.attributes('data-task-id'));
}

beforeEach(() => {
  fixture.expandTask.mockClear();
  fixture.syncCards.mockClear();
});
afterEach(() => {
  wrappers.splice(0).forEach((wrapper) => wrapper.unmount());
});

describe('task list bounded window', () => {
  it('keeps all 1000 seqs in the projection while mounting only the latest 60 cards', () => {
    const wrapper = setup();
    const window = wrapper.get('[data-testid="task-list-window"]');
    expect(window.attributes('data-task-total')).toBe('1000');
    expect(ids(wrapper)).toHaveLength(60);
    expect(ids(wrapper)[0]).toBe('task-0941');
    expect(ids(wrapper).at(-1)).toBe('task-1000');
    expect(wrapper.get('.task-card[data-task-seq="941"]').attributes('data-task-seq')).toBe('941');
    expect(wrapper.get('.task-card[data-task-seq="1000"]').attributes('data-task-seq')).toBe('1000');
  });

  it('pages earlier without duplicates, stops at seq 1, and pages back to seq 1000', async () => {
    const wrapper = setup();
    await wrapper.get('[data-testid="task-load-earlier"]').trigger('click');
    expect(ids(wrapper)).toHaveLength(60);
    expect(ids(wrapper)[0]).toBe('task-0911');
    expect(ids(wrapper).at(-1)).toBe('task-0970');
    expect(new Set(ids(wrapper)).size).toBe(60);
    expect(wrapper.get('[data-testid="task-list-window"]').attributes('data-task-total')).toBe('1000');

    for (let index = 0; index < 32; index += 1) {
      const earlier = wrapper.find('[data-testid="task-load-earlier"]');
      if (!earlier.exists()) break;
      await earlier.trigger('click');
    }
    expect(ids(wrapper)[0]).toBe('task-0001');
    expect(ids(wrapper)).toHaveLength(60);
    expect(wrapper.find('[data-testid="task-load-earlier"]').exists()).toBe(false);
    expect(new Set(ids(wrapper)).size).toBe(60);

    for (let index = 0; index < 32; index += 1) {
      const later = wrapper.find('[data-testid="task-load-later"]');
      if (!later.exists()) break;
      await later.trigger('click');
    }
    expect(ids(wrapper).at(-1)).toBe('task-1000');
    expect(ids(wrapper)).toHaveLength(60);
    expect(wrapper.find('[data-testid="task-load-later"]').exists()).toBe(false);
  });
});
