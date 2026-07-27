import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import MemberSessionPanel from '../MemberSessionPanel.vue';
import type { MemberSession } from '../../../../features/chat/v2Types';

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
});
