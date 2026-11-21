import { mount } from '@vue/test-utils';
import { defineComponent } from 'vue';
import { createI18n } from 'vue-i18n';
import { describe, expect, it } from 'vitest';
import zh from '../../../i18n/locales/zh-CN';
import TodayInbox from '../TodayInbox.vue';
import TodayMail from '../TodayMail.vue';
import TodayTasks from '../TodayTasks.vue';
import type { InboxItemRow } from '../../../features/approval/api';

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zh } });
// 切换按钮组：每个选项一个按钮，点了发 update:modelValue
const ToggleStub = defineComponent({
  props: { modelValue: { type: String, default: '' }, options: { type: Array, default: () => [] } },
  emits: ['update:modelValue'],
  template:
    '<div><button v-for="o in options" :key="o.value" :data-filter="o.value" @click="$emit(\'update:modelValue\', o.value)">{{ o.label }}</button></div>',
});
const stubs = {
  'q-btn-toggle': ToggleStub,
  'q-btn': true,
  'q-input': true,
  'q-checkbox': true,
  'router-link': { template: '<a><slot /></a>' },
};

const row = (id: string, extra: Partial<InboxItemRow>): InboxItemRow => ({
  id,
  source: 'task_review',
  kind: 'review',
  title: id,
  summary: '',
  risk_level: 'low',
  requester: '',
  agent_key: '',
  created_at: '',
  due_at: '',
  actions: ['approve'],
  reject_requires_note: false,
  link: '',
  context_json: '',
  ...extra,
});

describe('工作台今日 · 待我处理的筛选', () => {
  const rows = [
    row('普通', {}),
    row('高风险', { risk_level: 'high' }),
    row('今天到期', { due_at: new Date().toISOString() }),
    row('明年到期', { due_at: new Date(Date.now() + 366 * 86_400_000).toISOString() }),
  ];
  const mountInbox = () =>
    mount(TodayInbox, {
      props: {
        rows,
        total: 4,
        loaded: true,
        failed: false,
        degraded: [],
        busyIds: new Set<string>(),
        selectedId: '普通',
      },
      global: { plugins: [i18n], stubs },
    });
  const titles = (wrapper: ReturnType<typeof mountInbox>) =>
    wrapper.findAll('.today-inbox__row').map((li) => li.attributes('id'));

  it('默认全部；选「高风险」「今天到期」只留对应条目，并把选中项落到筛选结果的第一条', async () => {
    const wrapper = mountInbox();
    expect(titles(wrapper)).toHaveLength(4);

    await wrapper.get('[data-filter="high"]').trigger('click');
    expect(wrapper.findAll('.today-inbox__row')).toHaveLength(1);
    expect(wrapper.emitted('select')?.at(-1)).toEqual(['高风险']);

    await wrapper.get('[data-filter="due"]').trigger('click');
    expect(wrapper.findAll('.today-inbox__row')).toHaveLength(1);
    expect(wrapper.emitted('select')?.at(-1)).toEqual(['今天到期']);
  });

  it('筛选结果为空时写明，不留一块空白', async () => {
    const wrapper = mount(TodayInbox, {
      props: {
        rows: [rows[0]],
        total: 1,
        loaded: true,
        failed: false,
        degraded: [],
        busyIds: new Set<string>(),
        selectedId: '普通',
      },
      global: { plugins: [i18n], stubs },
    });
    await wrapper.get('[data-filter="high"]').trigger('click');
    expect(wrapper.text()).toContain('这个筛选下没有条目');
  });
});

describe('工作台今日 · 最新邮件与任务两张卡', () => {
  it('邮件：只显示发件人名字；没有权限、读取失败、没有邮件各自写明', () => {
    const mail = {
      id: 'm1',
      account_id: 'a',
      subject: '年框合同',
      sender: '王总 <wang@example.com>',
      snippet: '',
      received_at: '',
      task_ref: 't1',
      has_attachments: false,
      to: '',
      cc: '',
      seen: true,
      flagged: false,
      archived: false,
      folder: 'inbox',
      trashed: false,
      spam: false,
      labels: [],
      send_at: '',
    };
    const ok = mount(TodayMail, {
      props: { mails: [mail], total: 9, status: 'ok' },
      global: { plugins: [i18n], stubs },
    });
    expect(ok.get('.today-mail__from').text()).toBe('王总');
    expect(ok.text()).toContain('已建任务');
    const text = (status: 'denied' | 'error' | 'ok') =>
      mount(TodayMail, { props: { mails: [], total: 0, status }, global: { plugins: [i18n], stubs } }).text();
    expect(text('denied')).toContain('没有邮件权限');
    expect(text('error')).toContain('邮件读取失败');
    expect(text('ok')).toContain('还没有邮件');
  });

  it('同一个任务组件按生命周期分两块：我的任务只列待办（新任务在底部指令窗里写），进行中列我在做的和 AI 在做的', () => {
    const row = (id: string, status: string) => ({
      id,
      title: id,
      body: '',
      source: 'manual',
      source_ref: '',
      status,
      assignee_kind: '',
      assignee_id: '',
      assignee_name: '',
      session_id: '',
      due_at: '',
      file_count: 0,
      created_at: '2026-11-20T00:00:00Z',
      updated_at: '',
    });
    const base = {
      tasks: [row('写周报', 'open'), row('改合同', 'doing')],
      tasksStatus: 'ok' as const,
      taskBusy: new Set<string>(),
      lastDone: null,
      notes: [],
      delegated: [
        {
          id: 'office:t9',
          title: '竞品分析',
          agentKey: '分析师',
          state: 'running' as const,
          at: '',
          to: '/chat',
          officeTaskId: 't9',
          detail: '正在检索资料',
        },
      ],
      aiTasks: [],
      delegatedStatus: 'ok' as const,
      now: new Date('2026-11-20T08:00:00Z'),
    };
    const mountPart = (part: 'tasks' | 'delegated') =>
      mount(TodayTasks, {
        props: { ...base, part },
        global: { plugins: [i18n], stubs: { ...stubs, TaskRecipientMenu: true } },
      });
    const mine = mountPart('tasks');
    expect(mine.text()).toContain('我的任务');
    expect(mine.findAll('.tk-card__title').map((el) => el.text())).toEqual(['写周报']);
    expect(mine.find('textarea').exists()).toBe(false);
    expect(mine.find('[aria-label="新任务（在底部指令窗里写）"]').exists()).toBe(true);
    const doing = mountPart('delegated');
    expect(doing.text()).toContain('进行中');
    expect(doing.findAll('.tk-card__title').map((el) => el.text())).toEqual(['竞品分析', '改合同']);
    expect(doing.text()).toContain('正在检索资料');
    expect(doing.text()).toContain('实时');
  });
});
