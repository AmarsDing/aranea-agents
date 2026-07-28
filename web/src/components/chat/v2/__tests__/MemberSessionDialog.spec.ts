// web/src/components/chat/v2/__tests__/MemberSessionDialog.spec.ts
// MemberSessionDialog：Graph 节点成员行点击后弹出的对话内容弹框。
// MemberSessionPanel embedded 模式：弹框内始终展开、无折叠开关。
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import MemberSessionDialog from '../MemberSessionDialog.vue';
import MemberSessionPanel from '../MemberSessionPanel.vue';
import zhCN from '../../../../i18n/locales/zh-CN';
import type { MemberSession } from '../../../../features/chat/v2Types';

// 移动端断点开关（$q.screen.lt.sm）：默认桌面，测试可切换。
const mobileFlag = vi.hoisted(() => ({ ltSm: false }));
vi.mock('quasar', () => ({
  useQuasar: () => ({ screen: { lt: { sm: mobileFlag.ltSm } } }),
}));

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } });

const quasarStubs = {
  'q-dialog': {
    name: 'QDialog',
    props: ['modelValue', 'maximized'],
    emits: ['update:modelValue'],
    template: '<div v-if="modelValue" class="q-dialog-stub"><slot /></div>',
  },
  'q-card': { template: '<div class="q-card-stub"><slot /></div>' },
  'q-card-section': { template: '<div class="q-card-section-stub"><slot /></div>' },
  'q-btn': {
    // onClick 经 v-bind="$attrs" 自动注册为原生监听；再显式 @click 会触发两次
    template: '<button type="button" v-bind="$attrs"><slot /></button>',
  },
  'q-input': {
    props: ['modelValue'],
    emits: ['update:modelValue', 'keyup'],
    template:
      '<div class="q-input-stub"><input :value="modelValue" @input="$emit(\'update:modelValue\', ($event.target).value)" /><slot name="append" /></div>',
  },
  'q-icon': { template: '<i />' },
  'q-avatar': { template: '<span />' },
  'q-badge': { template: '<span class="q-badge-stub"><slot /></span>' },
};

function baseMember(overrides: Partial<MemberSession> = {}): MemberSession {
  return {
    ID: 'ms1',
    TeamRunID: 'tr1',
    TeamStageID: 'ts1',
    TaskID: 'tk1',
    SessionID: 'ms-sess',
    SpiritSessionID: 's1',
    AgentKey: 'coder',
    AgentName: 'Coder',
    AvatarURL: '',
    Status: 'running',
    Seq: 1,
    Version: 1,
    StartedAt: new Date().toISOString(),
    FinishedAt: null,
    Error: '',
    ...overrides,
  };
}

describe('MemberSessionDialog', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    mobileFlag.ltSm = false;
  });

  it('renders nothing when closed', () => {
    const wrapper = mount(MemberSessionDialog, {
      props: { open: false, memberSession: baseMember() },
      global: { plugins: [i18n], stubs: quasarStubs },
    });
    expect(wrapper.find('.q-dialog-stub').exists()).toBe(false);
  });

  it('renders embedded MemberSessionPanel with member name in header when open', () => {
    const wrapper = mount(MemberSessionDialog, {
      props: { open: true, memberSession: baseMember({ AgentName: '阿尔法' }) },
      global: { plugins: [i18n], stubs: quasarStubs },
    });
    expect(wrapper.find('.member-session-dialog__title').text()).toContain('阿尔法');
    expect(wrapper.findComponent(MemberSessionPanel).exists()).toBe(true);
    expect(wrapper.findComponent(MemberSessionPanel).props('embedded')).toBe(true);
  });

  it('emits update:open(false) when the close button is clicked', async () => {
    const wrapper = mount(MemberSessionDialog, {
      props: { open: true, memberSession: baseMember() },
      global: { plugins: [i18n], stubs: quasarStubs },
    });
    await wrapper.find('.member-session-dialog__close').trigger('click');
    expect(wrapper.emitted('update:open')?.[0]).toEqual([false]);
  });

  it('passes through pause-agent / inject-agent / expand / confirm-step events', async () => {
    const wrapper = mount(MemberSessionDialog, {
      props: { open: true, memberSession: baseMember() },
      global: { plugins: [i18n], stubs: quasarStubs },
    });
    const panel = wrapper.findComponent(MemberSessionPanel);
    panel.vm.$emit('pause-agent', 'ms-sess');
    panel.vm.$emit('inject-agent', { sessionId: 'ms-sess', message: 'hi' });
    panel.vm.$emit('expand', ['ms-sess']);
    panel.vm.$emit('confirm-step', { stepId: 'st1', approved: true });
    expect(wrapper.emitted('pause-agent')?.[0]).toEqual(['ms-sess']);
    expect(wrapper.emitted('inject-agent')?.[0]).toEqual([{ sessionId: 'ms-sess', message: 'hi' }]);
    expect(wrapper.emitted('expand')?.[0]).toEqual([['ms-sess']]);
    expect(wrapper.emitted('confirm-step')?.[0]).toEqual([{ stepId: 'st1', approved: true }]);
  });

  // 移动端触控化（72 §3.2）：窄屏弹框最大化，注入输入栏成为底部操作区。
  it('is not maximized on desktop (screen.lt.sm = false)', () => {
    const wrapper = mount(MemberSessionDialog, {
      props: { open: true, memberSession: baseMember() },
      global: { plugins: [i18n], stubs: quasarStubs },
    });
    expect(wrapper.findComponent({ name: 'QDialog' }).props('maximized')).toBeFalsy();
  });

  it('is maximized on mobile (screen.lt.sm = true)', () => {
    mobileFlag.ltSm = true;
    const wrapper = mount(MemberSessionDialog, {
      props: { open: true, memberSession: baseMember() },
      global: { plugins: [i18n], stubs: quasarStubs },
    });
    expect(wrapper.findComponent({ name: 'QDialog' }).props('maximized')).toBe(true);
  });
});

describe('MemberSessionPanel embedded mode', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('is always expanded without collapse toggle, even for terminal status', () => {
    const wrapper = mount(MemberSessionPanel, {
      props: { memberSession: baseMember({ Status: 'completed' }), embedded: true },
      global: { plugins: [i18n], stubs: quasarStubs },
    });
    // 无折叠箭头
    expect(wrapper.find('.member-header__icon').exists()).toBe(false);
    // 内容始终可见
    expect(wrapper.find('.member-body').isVisible()).toBe(true);
    // 挂载即请求懒加载（completed 状态默认折叠的常规面板不会 emit）
    expect(wrapper.emitted('expand')?.[0]).toEqual([['ms-sess']]);
  });

  it('header click does not collapse in embedded mode', async () => {
    const wrapper = mount(MemberSessionPanel, {
      props: { memberSession: baseMember(), embedded: true },
      global: { plugins: [i18n], stubs: quasarStubs },
    });
    await wrapper.find('.member-header').trigger('click');
    expect(wrapper.find('.member-body').isVisible()).toBe(true);
  });
});
