import { afterEach, describe, expect, it } from 'vitest';
import { mount, type VueWrapper } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import RunTrack from '../RunTrack.vue';

const stubs = {
  'q-icon': { template: '<i />' },
  'q-btn': {
    props: ['label', 'icon'],
    emits: ['click'],
    template: '<button type="button" v-bind="$attrs" @click="$emit(\'click\')">{{ label || icon }}</button>',
  },
  'q-skeleton': { template: '<div class="skeleton" />' },
};

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  messages: {
    'zh-CN': {
      shell: { state: { emptyTitle: '暂无数据', emptyDesc: '', loading: '加载中' } },
      monitorPage: {
        traces: {
          runTrack: {
            title: '运行轨迹',
            steps: '{n} 个步骤',
            failed: '{n} 个失败',
            total: '总耗时 {value}',
            jumpFirstFailure: '定位首个失败步骤',
            emptyTitle: '本次运行没有步骤数据',
            emptyDesc: '后端未记录 span，可查看原始 JSON',
            noKind: '步骤',
            toneError: '失败',
            toneOk: '成功',
            toneRunning: '运行中',
            toneWarn: '超时',
            toneIdle: '未知',
          },
        },
      },
    },
  },
});

const spans = [
  { id: 'root', name: 'agent.run', kind: 'agent', status: 'ok', start_ms: 0, duration_ms: 600 },
  { id: 'tool', parent_id: 'root', name: 'tool.search', kind: 'tool', status: 'error', start_ms: 100, duration_ms: 200, error: 'HTTP 500' },
  { id: 'retry', parent_id: 'root', name: 'tool.retry', kind: 'tool', status: 'ok', start_ms: 400, duration_ms: 100 },
];

const wrappers: VueWrapper[] = [];
function track(props: Record<string, unknown> = {}) {
  const wrapper = mount(RunTrack, {
    attachTo: document.body,
    props: { spans, ...props },
    global: { plugins: [i18n], stubs },
  });
  wrappers.push(wrapper);
  return wrapper;
}

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount();
  document.body.innerHTML = '';
});

describe('RunTrack', () => {
  it('按时间顺序渲染每一步，摘要含步骤数 / 失败数 / 总耗时（aria-live）', () => {
    const wrapper = track();
    const summary = wrapper.get('[data-test="run-track-summary"]');

    expect(summary.text()).toContain('3 个步骤');
    expect(summary.text()).toContain('1 个失败');
    expect(summary.text()).toContain('总耗时 600ms');
    expect(summary.attributes('aria-live')).toBe('polite');

    const steps = wrapper.findAll('[data-test="run-track-step"]');
    expect(steps.map((s) => s.attributes('data-step-id'))).toEqual(['root', 'tool', 'retry']);
    expect(steps[1].attributes('data-tone')).toBe('error');
  });

  it('「定位首个失败步骤」点击后上抛 select（失败步骤 id）', async () => {
    const wrapper = track();

    await wrapper.get('[data-test="run-track-jump"]').trigger('click');
    expect(wrapper.emitted('select')).toEqual([['tool']]);
  });

  it('无失败步骤时不渲染定位按钮', () => {
    const wrapper = track({ spans: [{ id: 'a', name: 'ok', status: 'ok', start_ms: 0, duration_ms: 10 }] });

    expect(wrapper.find('[data-test="run-track-jump"]').exists()).toBe(false);
  });

  it('点击步骤与键盘 Enter/Space 都上抛 select，selectedId 标 aria-current', async () => {
    const wrapper = track({ selectedId: 'tool' });

    const step = wrapper.get('[data-test="run-track-step"][data-step-id="retry"]');
    await step.trigger('click');
    await step.trigger('keydown', { key: 'Enter' });
    await step.trigger('keydown', { key: ' ' });

    expect(wrapper.emitted('select')).toEqual([['retry'], ['retry'], ['retry']]);
    expect(wrapper.get('[data-step-id="tool"]').attributes('aria-current')).toBe('true');
    expect(wrapper.get('[data-step-id="retry"]').attributes('aria-current')).toBeUndefined();
  });

  it('错误步骤把错误信息放进可访问名称，并渲染在步骤行内', () => {
    const wrapper = track();

    const step = wrapper.get('[data-step-id="tool"]');
    expect(step.attributes('aria-label')).toContain('HTTP 500');
    expect(step.text()).toContain('HTTP 500');
    expect(step.text()).toContain('失败');
  });

  it('空 spans 渲染空态（不渲染步骤列表，也不渲染定位按钮）', () => {
    const wrapper = track({ spans: [] });

    expect(wrapper.findAll('[data-test="run-track-step"]')).toHaveLength(0);
    expect(wrapper.find('[data-test="run-track-jump"]').exists()).toBe(false);
    expect(wrapper.text()).toContain('本次运行没有步骤数据');
  });

  it('loading 时 busy 且走骨架，不误报空态', () => {
    const wrapper = track({ spans: [], loading: true });

    expect(wrapper.get('[data-test="run-track"]').attributes('aria-busy')).toBe('true');
    expect(wrapper.text()).not.toContain('本次运行没有步骤数据');
  });
});
