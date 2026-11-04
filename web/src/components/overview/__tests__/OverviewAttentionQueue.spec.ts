import { mount } from '@vue/test-utils';
import { defineComponent } from 'vue';
import { createI18n } from 'vue-i18n';
import { describe, expect, it } from 'vitest';
import zh from '../../../i18n/locales/zh-CN';
import OverviewAttentionQueue from '../OverviewAttentionQueue.vue';
import type { AttentionItem, AttentionLaneState } from '../../../features/usage/types';

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zh } });
const QBtnStub = defineComponent({
  inheritAttrs: false,
  props: { label: { type: String, default: '' } },
  emits: ['click'],
  template: '<button v-bind="$attrs" @click="$emit(\'click\')">{{ label }}</button>',
});
const items: AttentionItem[] = Array.from({ length: 5 }, (_, index) => ({
  id: `hitl:approval-${index}`,
  lane: 'backlog',
  severity: 'warning',
  title: index === 4 ? '未关联审批' : `审批 ${index}`,
  detail: '需要人工确认',
  occurred_at: '',
  ...(index < 4 ? { to: { path: '/task-workspace' } } : {}),
}));
const lane: AttentionLaneState = { lane: 'backlog', status: 'ready', items, count: 5 };

describe('OverviewAttentionQueue full backlog', () => {
  it('expands inline and marks unassociated approvals without guessing a link', async () => {
    const wrapper = mount(OverviewAttentionQueue, {
      props: { lanes: [lane] },
      global: { plugins: [i18n], stubs: { 'q-icon': true, 'q-btn': QBtnStub } },
    });
    expect(wrapper.findAll('.ov-lane__items > li')).toHaveLength(3);
    const toggle = wrapper.get('[data-testid="attention-toggle-backlog"]');
    expect(toggle.attributes('aria-expanded')).toBe('false');
    await toggle.trigger('click');
    expect(wrapper.findAll('.ov-lane__items > li')).toHaveLength(5);
    expect(toggle.attributes('aria-expanded')).toBe('true');
    expect(wrapper.get('[data-testid="attention-unassociated-hitl:approval-4"]').text()).toBe('未关联');
    expect(wrapper.findAll('button.ov-item')).toHaveLength(4);
  });
});
