import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, shallowMount, type VueWrapper } from '@vue/test-utils';
import CodingTasksPage from './CodingTasksPage.vue';
import AppRegistryInspector from '../components/layout/AppRegistryInspector.vue';
import type { CodingTask } from '../features/coding/api';

const { task } = vi.hoisted(() => ({
  task: {
    id: 'task-1',
    agentKey: 'claude-code',
    projectName: 'aranea-agents',
    prompt: '修复登录校验\n保留现有错误提示',
    status: 'succeeded',
    sessionId: undefined,
    summary: undefined,
    error: undefined,
    progressCount: undefined,
    createdAt: '2026-11-10T08:00:00Z',
    completedAt: undefined,
    childPid: undefined,
    lastActiveAt: undefined,
    lastExcerpt: undefined,
    heartbeatMiss: undefined,
    livenessState: undefined,
    origin: undefined,
    reviewStatus: undefined,
    reviewComment: undefined,
    reviewedAt: undefined,
  } satisfies CodingTask,
}));

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ push: vi.fn() }),
}));

vi.mock('quasar', () => ({
  useQuasar: () => ({ notify: vi.fn() }),
}));

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>();
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  };
});

vi.mock('../features/coding/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../features/coding/api')>();
  return {
    ...actual,
    listCodingAgents: vi.fn().mockResolvedValue([
      { agentKey: 'claude-code', displayName: 'Claude Code', enabled: true },
    ]),
    listCodingProjects: vi.fn().mockResolvedValue([{ name: 'aranea-agents', path: 'E:/teamwork/aranea-agents' }]),
    listCodingTasks: vi.fn().mockResolvedValue([task]),
  };
});

const wrappers: VueWrapper[] = [];

afterEach(() => {
  wrappers.splice(0).forEach((wrapper) => wrapper.unmount());
});

describe('CodingTasksPage inspector', () => {
  it('opens the inspector for the selected coding task', async () => {
    const wrapper = shallowMount(CodingTasksPage, {
      global: {
        directives: { ripple: {} },
        stubs: {
          'q-page': { template: '<div><slot /></div>' },
          ...Object.fromEntries(
          [
            'q-btn', 'q-icon', 'q-banner', 'q-card', 'q-card-section',
            'q-select', 'q-input', 'q-expansion-item', 'q-tabs', 'q-tab',
            'q-avatar', 'q-badge', 'q-dialog', 'q-separator', 'q-list',
            'q-item', 'q-item-section', 'q-item-label',
          ].map((name) => [name, true]),
          ),
        },
      },
    });
    wrappers.push(wrapper);
    await flushPromises();

    expect(wrapper.html()).toContain('coding-task-card');
    await wrapper.findComponent({ name: 'CodingTaskCard' }).vm.$emit('inspect', task);
    await flushPromises();

    const inspector = wrapper.findComponent(AppRegistryInspector);
    expect(inspector.exists()).toBe(true);
    expect(inspector.props('modelValue')).toBe(true);
    expect(inspector.props('title')).toBe('修复登录校验');
    expect(inspector.props('eyebrow')).toBe('TASK / task-1');
  });
});
