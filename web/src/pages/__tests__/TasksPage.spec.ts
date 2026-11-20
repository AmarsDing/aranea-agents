import { flushPromises, mount } from '@vue/test-utils';
import { ref } from 'vue';
import { createI18n } from 'vue-i18n';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import zh from '../../i18n/locales/zh-CN';
import { dayOrClock, type DelegatedRow } from '../../features/today/logic';
import type { OfficeTaskRow } from '../../features/office/types';
import TasksPage from '../TasksPage.vue';

const task = (id: string, extra: Partial<OfficeTaskRow> = {}): OfficeTaskRow => ({
  id,
  title: id,
  body: '',
  source: 'manual',
  source_ref: '',
  status: 'open',
  assignee_kind: '',
  assignee_id: '',
  assignee_name: '',
  session_id: '',
  due_at: '',
  file_count: 0,
  created_at: '2026-11-01T00:00:00Z',
  updated_at: '2026-11-01T00:00:00Z',
  ...extra,
});
const run = (id: string, state: DelegatedRow['state'], officeTaskId = ''): DelegatedRow => ({
  id,
  title: id,
  agentKey: '写手',
  state,
  at: '',
  to: `/chat?session=${id}`,
  officeTaskId,
  detail: '',
});

const api = vi.hoisted(() => ({ listOfficeTasks: vi.fn(), setOfficeTaskStatus: vi.fn() }));
vi.mock('../../features/office/api', () => api);
vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ replace: vi.fn() }),
}));

const side = {
  tasks: ref<OfficeTaskRow[]>([]),
  tasksStatus: ref('ok'),
  taskBusy: ref(new Set<string>()),
  notes: ref<OfficeTaskRow[]>([]),
  delegated: ref<DelegatedRow[]>([]),
  aiTasks: ref<OfficeTaskRow[]>([]),
  delegatedStatus: ref('ok'),
  refresh: vi.fn(async () => undefined),
  addTask: vi.fn(async () => ({ error: '', task: null })),
  addNote: vi.fn(async () => ''),
  ingest: vi.fn(),
};
vi.mock('../../features/today/useTodaySide', () => ({ useTodaySide: () => side }));

// 拖动库换成按列表平铺的桩：只关心卡片落在哪一栏
const draggableStub = {
  props: ['list'],
  template:
    '<ul><li v-for="element in list" :key="element.key"><slot name="item" :element="element" /></li><slot name="footer" /></ul>',
};

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zh } });
const mountPage = async () => {
  const wrapper = mount(TasksPage, {
    global: {
      plugins: [i18n],
      stubs: {
        'q-page': { template: '<div><slot /></div>' },
        draggable: draggableStub,
        TaskComposer: true,
        TaskDetailDrawer: true,
        TaskRecipientMenu: true,
        'router-link': { props: ['to'], template: '<a :href="to"><slot /></a>' },
      },
    },
  });
  await flushPromises();
  return wrapper;
};
const column = (wrapper: Awaited<ReturnType<typeof mountPage>>, key: string) =>
  wrapper.get(`[aria-labelledby="tk-col-${key}"]`);
const titles = (wrapper: Awaited<ReturnType<typeof mountPage>>, key: string) =>
  column(wrapper, key)
    .findAll('.tk-card__title')
    .map((el) => el.text());

describe('工作台 · 任务页（生命周期四栏）', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    side.tasks.value = [task('写周报'), task('改合同', { status: 'doing' })];
    side.aiTasks.value = [
      task('竞品分析', { assignee_kind: 'agent', assignee_id: 'ag', session_id: 's1' }),
      task('出海报', { assignee_kind: 'agent', assignee_id: 'ag', session_id: 's2' }),
    ];
    side.delegated.value = [
      run('卡住的编码', 'stuck'),
      run('出海报', 'review', '出海报'),
      run('竞品分析', 'running', '竞品分析'),
    ];
    api.listOfficeTasks.mockImplementation(async (_status: string, _limit: number, source = '') =>
      source === 'delegate'
        ? [task('交办的', { status: 'done', assignee_kind: 'agent', updated_at: '2026-11-03T00:00:00Z' })]
        : [task('我做的', { status: 'done', updated_at: '2026-11-02T00:00:00Z' })],
    );
    api.setOfficeTaskStatus.mockResolvedValue(task('x'));
  });

  it('按生命周期归栏：待办 / 进行中（AI 的排前，再是我在做的）/ 待验收 / 已完成按时间倒排', async () => {
    const wrapper = await mountPage();
    expect(titles(wrapper, 'todo')).toEqual(['写周报']);
    expect(titles(wrapper, 'doing')).toEqual(['卡住的编码', '竞品分析', '改合同']);
    expect(titles(wrapper, 'review')).toEqual(['出海报']);
    expect(titles(wrapper, 'done')).toEqual(['交办的', '我做的']);
    expect(column(wrapper, 'doing').text()).toContain('实时');
  });

  it('卡片动作走生命周期：开始 → doing，完成 → done，验收 → done，重开 → open，并重新读取各栏', async () => {
    const wrapper = await mountPage();
    const card = (col: string, name: string) =>
      column(wrapper, col)
        .findAll('.tk-card')
        .find((el) => el.text().includes(name))!;

    await card('todo', '写周报').get('[aria-label="开始做：写周报"]').trigger('click');
    await flushPromises();
    expect(api.setOfficeTaskStatus).toHaveBeenLastCalledWith('写周报', 'doing');

    await card('doing', '改合同').get('[aria-label="完成：改合同"]').trigger('click');
    await flushPromises();
    expect(api.setOfficeTaskStatus).toHaveBeenLastCalledWith('改合同', 'done');

    await card('review', '出海报').get('[aria-label="验收通过：出海报"]').trigger('click');
    await flushPromises();
    expect(api.setOfficeTaskStatus).toHaveBeenLastCalledWith('出海报', 'done');

    await card('done', '我做的').get('[aria-label="重开：我做的"]').trigger('click');
    await flushPromises();
    expect(api.setOfficeTaskStatus).toHaveBeenLastCalledWith('我做的', 'open');
    expect(side.refresh).toHaveBeenCalledTimes(4);
  });

  it('两次读取都失败时写明读取失败，不显示成「还没有完成的任务」', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => undefined);
    api.listOfficeTasks.mockRejectedValue(new Error('boom'));
    const wrapper = await mountPage();
    expect(column(wrapper, 'done').get('[role="alert"]').text()).toBe('已完成的任务读取失败。');
  });
});

describe('任务时间：当天写钟点，其余写月日', () => {
  it('dayOrClock', () => {
    const now = new Date(2026, 10, 19, 15, 0);
    expect(dayOrClock(new Date(2026, 10, 19, 9, 5).toISOString(), now)).toBe('09:05');
    expect(dayOrClock(new Date(2026, 10, 3, 9, 5).toISOString(), now)).toBe('11-03');
    expect(dayOrClock('', now)).toBe('');
  });
});
