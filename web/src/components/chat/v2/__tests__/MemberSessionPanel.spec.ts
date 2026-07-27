import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import MemberSessionPanel from '../MemberSessionPanel.vue';
import type { MemberSession, Task } from '../../../../features/chat/v2Types';
import { useChatActivityStore } from '../../../../stores/chat/activityV2Store';

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

function memberTask(overrides: Partial<Task> = {}): Task {
  return {
    ID: 'task-ms-1',
    SessionID: 'ms-sess',
    UserMessage: '调研医疗云市场趋势并输出简报',
    Status: 'completed',
    Seq: 1,
    Version: 1,
    CreatedAt: new Date().toISOString(),
    UpdatedAt: new Date().toISOString(),
    CompletedAt: null,
    ...overrides,
  };
}

const quasarStubs = {
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
  'q-badge': { template: '<span><slot /></span>' },
};

describe('MemberSessionPanel', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('emits pause-agent with chat SessionID (not entity ID)', async () => {
    const wrapper = mount(MemberSessionPanel, {
      props: { memberSession: baseMember() },
      global: { stubs: quasarStubs },
    });
    expect(wrapper.find('.member-input-bar').exists()).toBe(true);
    const stopBtn = wrapper.find('.member-input-bar button');
    expect(stopBtn.exists()).toBe(true);
    await stopBtn.trigger('click');
    expect(wrapper.emitted('pause-agent')?.[0]).toEqual(['ms-sess']);
  });

  it('emits inject-agent with chat SessionID', async () => {
    const wrapper = mount(MemberSessionPanel, {
      props: { memberSession: baseMember() },
      global: { stubs: quasarStubs },
    });
    const input = wrapper.find('.member-input-bar input');
    expect(input.exists()).toBe(true);
    await input.setValue('hello agent');
    await wrapper.vm.$nextTick();
    const sendBtn = wrapper.find('.member-input-bar button');
    expect(sendBtn.exists()).toBe(true);
    await sendBtn.trigger('click');
    expect(wrapper.emitted('inject-agent')?.[0]).toEqual([{ sessionId: 'ms-sess', message: 'hello agent' }]);
  });

  it('emits expand with SessionID when mounting expanded (running)', () => {
    const wrapper = mount(MemberSessionPanel, {
      props: { memberSession: baseMember({ Status: 'running' }) },
      global: { stubs: quasarStubs },
    });
    expect(wrapper.emitted('expand')?.[0]).toEqual([['ms-sess']]);
  });

  it('hides input bar for system agent keys', () => {
    const wrapper = mount(MemberSessionPanel, {
      props: { memberSession: baseMember({ AgentKey: '__spirit__' }) },
      global: { stubs: quasarStubs },
    });
    expect(wrapper.find('.member-input-bar').exists()).toBe(false);
  });

  // ── B（2026-07-27）：终态成员也可「补充内容再执行」──
  it('shows input bar for completed member (终态补充再执行)', () => {
    const wrapper = mount(MemberSessionPanel, {
      props: { memberSession: baseMember({ Status: 'completed' }) },
      global: { stubs: quasarStubs },
    });
    expect(wrapper.find('.member-input-bar').exists()).toBe(true);
  });

  it('shows input bar for failed member', () => {
    const wrapper = mount(MemberSessionPanel, {
      props: { memberSession: baseMember({ Status: 'failed' }) },
      global: { stubs: quasarStubs },
    });
    expect(wrapper.find('.member-input-bar').exists()).toBe(true);
  });

  it('hides input bar for skipped member', () => {
    const wrapper = mount(MemberSessionPanel, {
      props: { memberSession: baseMember({ Status: 'skipped' }) },
      global: { stubs: quasarStubs },
    });
    expect(wrapper.find('.member-input-bar').exists()).toBe(false);
  });

  it('completed member: inject emits with SessionID', async () => {
    const wrapper = mount(MemberSessionPanel, {
      props: { memberSession: baseMember({ Status: 'completed' }) },
      global: { stubs: quasarStubs },
    });
    const input = wrapper.find('.member-input-bar input');
    await input.setValue('补充一下数据来源');
    await wrapper.vm.$nextTick();
    await wrapper.find('.member-input-bar button').trigger('click');
    expect(wrapper.emitted('inject-agent')?.[0]).toEqual([
      { sessionId: 'ms-sess', message: '补充一下数据来源' },
    ]);
  });

  // ── C（2026-07-27）：弹框显示成员收到的任务指令（输入内容）──
  it('shows task instruction block with member UserMessage', () => {
    const store = useChatActivityStore();
    store.upsertTask(memberTask());
    const wrapper = mount(MemberSessionPanel, {
      props: { memberSession: baseMember(), embedded: true },
      global: { stubs: quasarStubs },
    });
    const block = wrapper.find('.member-instruction');
    expect(block.exists()).toBe(true);
    expect(block.text()).toContain('调研医疗云市场趋势并输出简报');
  });

  it('collapses long instruction (>500 chars) by default, expands on click', async () => {
    const store = useChatActivityStore();
    store.upsertTask(memberTask({ UserMessage: '长指令'.repeat(300) }));
    const wrapper = mount(MemberSessionPanel, {
      props: { memberSession: baseMember(), embedded: true },
      global: { stubs: quasarStubs },
    });
    const text = wrapper.find('.member-instruction__text');
    expect(text.exists()).toBe(true);
    // 折叠态：不显示全文
    expect(text.text().length).toBeLessThan('长指令'.repeat(300).length);
    const toggle = wrapper.find('.member-instruction__toggle');
    expect(toggle.exists()).toBe(true);
    await toggle.trigger('click');
    expect(wrapper.find('.member-instruction__text').text()).toContain('长指令'.repeat(300));
  });

  it('hides instruction block when no task found', () => {
    const wrapper = mount(MemberSessionPanel, {
      props: { memberSession: baseMember(), embedded: true },
      global: { stubs: quasarStubs },
    });
    expect(wrapper.find('.member-instruction').exists()).toBe(false);
  });
});
