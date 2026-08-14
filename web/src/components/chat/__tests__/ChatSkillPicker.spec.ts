// web/src/components/chat/__tests__/ChatSkillPicker.spec.ts
// Skill 选择器：输入框工具条按钮 + 弹出面板多选卡片。
// 交互契约：点卡片 toggle（面板不关闭）、清空按钮仅在有选中时显示、
// 角标显示选中数、catalog 为空时整个入口隐藏。
import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import ChatSkillPicker from '../ChatSkillPicker.vue';
import zhCN from '../../../i18n/locales/zh-CN';
import type { SkillCatalogEntry } from '../../../features/skills/types';

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } });

const quasarStubs = {
  'q-btn': {
    emits: ['click'],
    template: '<button class="q-btn-stub" @click="$emit(\'click\')"><slot /></button>',
  },
  'q-icon': { template: '<i />' },
  'q-badge': { template: '<span class="q-badge-stub"><slot /></span>' },
  'q-tooltip': { template: '<span />' },
  'q-menu': { template: '<div class="q-menu-stub"><slot /></div>' },
};

function makeSkill(overrides: Partial<SkillCatalogEntry>): SkillCatalogEntry {
  return {
    slug: 'skill-x',
    name: '',
    description: '',
    tags: [],
    ...overrides,
  };
}

const catalog: SkillCatalogEntry[] = [
  makeSkill({ slug: 'alert-diagnosis', name: '告警诊断', description: '告警上下文组装 → 严重度评估' }),
  makeSkill({ slug: 'cms-alert', name: 'CMS 告警规则', description: '创建和查询云监控告警规则' }),
];

function mountPicker(props: { skills: SkillCatalogEntry[]; selectedSlugs: string[] }) {
  return mount(ChatSkillPicker, {
    props,
    global: { plugins: [i18n], stubs: quasarStubs },
  });
}

describe('ChatSkillPicker', () => {
  it('catalog 为空时隐藏整个入口', () => {
    const wrapper = mountPicker({ skills: [], selectedSlugs: [] });
    expect(wrapper.find('.skill-picker').exists()).toBe(false);
  });

  it('渲染 skill 卡片：名称 + 简介', () => {
    const wrapper = mountPicker({ skills: catalog, selectedSlugs: [] });
    const cards = wrapper.findAll('.skill-picker-card');
    expect(cards).toHaveLength(2);
    expect(wrapper.text()).toContain('告警诊断');
    expect(wrapper.text()).toContain('告警上下文组装');
    expect(wrapper.text()).toContain('CMS 告警规则');
  });

  it('点击卡片 emit toggle（面板保持打开，可连续多选）', async () => {
    const wrapper = mountPicker({ skills: catalog, selectedSlugs: [] });
    const cards = wrapper.findAll('.skill-picker-card');

    await cards[0].trigger('click');
    expect(wrapper.emitted('toggle')).toEqual([['alert-diagnosis']]);

    await cards[1].trigger('click');
    expect(wrapper.emitted('toggle')).toEqual([['alert-diagnosis'], ['cms-alert']]);
  });

  it('选中卡片带选中态样式，未选中不带', () => {
    const wrapper = mountPicker({ skills: catalog, selectedSlugs: ['cms-alert'] });
    const cards = wrapper.findAll('.skill-picker-card');
    expect(cards[0].classes()).not.toContain('skill-picker-card--selected');
    expect(cards[1].classes()).toContain('skill-picker-card--selected');
  });

  it('角标显示选中数量，无选中时不显示角标', () => {
    const withSel = mountPicker({ skills: catalog, selectedSlugs: ['a', 'b'] });
    expect(withSel.find('.q-badge-stub').exists()).toBe(true);
    expect(withSel.find('.q-badge-stub').text()).toBe('2');

    const noSel = mountPicker({ skills: catalog, selectedSlugs: [] });
    expect(noSel.find('.q-badge-stub').exists()).toBe(false);
  });

  it('有选中时显示清空按钮并 emit clear；无选中时隐藏', async () => {
    const withSel = mountPicker({ skills: catalog, selectedSlugs: ['a'] });
    const clearBtn = withSel.find('.skill-picker__clear');
    expect(clearBtn.exists()).toBe(true);
    await clearBtn.trigger('click');
    expect(withSel.emitted('clear')).toHaveLength(1);

    const noSel = mountPicker({ skills: catalog, selectedSlugs: [] });
    expect(noSel.find('.skill-picker__clear').exists()).toBe(false);
  });
});
