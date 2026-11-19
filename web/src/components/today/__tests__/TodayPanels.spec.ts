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

  it('同一个任务组件按 part 只出一半：我的任务，或交办中', () => {
    const base = {
      tasks: [],
      tasksStatus: 'ok' as const,
      taskBusy: new Set<string>(),
      lastDone: null,
      notes: [],
      delegated: [],
      delegatedStatus: 'ok' as const,
      add: async () => '',
    };
    const part = (p?: 'tasks' | 'delegated') =>
      mount(TodayTasks, { props: { ...base, part: p }, global: { plugins: [i18n], stubs } }).text();
    expect(part('tasks')).toContain('我的任务');
    expect(part('tasks')).not.toContain('交办中');
    expect(part('delegated')).toContain('交办中');
    expect(part('delegated')).not.toContain('我的任务');
    expect(part()).toContain('我的任务');
    expect(part()).toContain('交办中');
  });
});
