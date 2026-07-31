// TeamEditorDialog — M53 Phase 11 F2：source 警告条 / 重置为派生 / 关联 Graph 选择器 /
// enable_checkpoint 开关 / custom 覆盖确认。
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { reactive, nextTick } from 'vue';
import TeamEditorDialog from '../TeamEditorDialog.vue';
import zhCN from '../../../i18n/locales/zh-CN';
import type { TeamDefinition } from '../../../features/teams/types';

// compile preview 走网络：mock 掉，避免测试环境真实请求。
vi.mock('../../../features/orchestration/compileApi', () => ({
  compileTeamGraph: vi.fn().mockResolvedValue({ definition_graph_json: '', graph_json: '' }),
}));

// $q.dialog 捕获（重置为派生 / 覆盖确认两条链路）
const dialogOnOk = vi.hoisted(() => ({ current: null as null | (() => void) }));
vi.mock('quasar', () => ({
  useQuasar: () => ({
    dialog: (opts: { onOk: (cb: () => void) => void }) => ({
      onOk: (cb: () => void) => {
        dialogOnOk.current = cb;
        return opts;
      },
    }),
    dark: { isActive: false },
  }),
}));

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } });

const quasarStubs = {
  'q-dialog': {
    name: 'QDialog',
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template: '<div v-if="modelValue" class="q-dialog-stub"><slot /></div>',
  },
  'q-card': { template: '<div class="q-card-stub"><slot /></div>' },
  'q-card-section': { template: '<div class="q-card-section-stub"><slot /></div>' },
  'q-card-actions': { template: '<div class="q-card-actions-stub"><slot /></div>' },
  'q-banner': { template: '<div class="q-banner-stub"><slot /><slot name="action" /></div>' },
  'q-btn': {
    props: ['label', 'disable'],
    template: '<button type="button" v-bind="$attrs"><slot />{{ label }}</button>',
  },
  'q-icon': { template: '<i />' },
  'q-badge': { template: '<span class="q-badge-stub" />' },
  'q-select': {
    name: 'QSelect',
    props: ['modelValue', 'options', 'label'],
    emits: ['update:modelValue'],
    template: '<div class="q-select-stub" v-bind="$attrs" />',
  },
  'q-input': { template: '<div class="q-input-stub" />' },
  'q-field': { template: '<div class="q-field-stub"><slot name="control" /></div>' },
  'q-toggle': {
    name: 'QToggle',
    props: ['modelValue', 'label'],
    emits: ['update:modelValue'],
    template:
      '<label class="q-toggle-stub" v-bind="$attrs"><input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', ($event.target).checked)" />{{ label }}</label>',
  },
  'q-expansion-item': { template: '<div class="q-expansion-stub"><slot /></div>' },
  'q-item': { template: '<div><slot /></div>' },
  'q-item-section': { template: '<div><slot /></div>' },
  'q-item-label': { template: '<div><slot /></div>' },
  'q-space': { template: '<span />' },
  'q-tooltip': { template: '<span />' },
  TeamCompilePreview: true,
};

function mkDefinition(overrides: Partial<TeamDefinition> = {}): TeamDefinition {
  return {
    version: 1,
    description: '',
    mode: 'sequential',
    max_concurrency: 2,
    timeout_seconds: 600,
    loop_max_iterations: 0,
    members: [{ agent_id: 'a1', role: 'worker', name: 'W', enabled: true, sort_order: 10 }],
    ...overrides,
  };
}

function mountDialog(definitionOverrides: Partial<TeamDefinition> = {}, props: Record<string, unknown> = {}) {
  const definition = reactive(mkDefinition(definitionOverrides));
  const form = reactive({
    team_key: 'demo',
    display_name: 'Demo',
    status: 'active',
    app_name: 'demo',
    taxonomy_industry_id: '',
    spirit_session_id: '',
  });
  const wrapper = mount(TeamEditorDialog, {
    props: {
      modelValue: true,
      editingId: 't1',
      form,
      definition,
      agentOptions: [{ label: 'W', value: 'a1' }],
      industryOptions: [],
      saving: false,
      canSave: true,
      isDark: false,
      ...props,
    },
    global: { plugins: [i18n], stubs: quasarStubs },
  });
  return { wrapper, definition };
}

describe('TeamEditorDialog F2 (M53 Phase 11)', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    dialogOnOk.current = null;
  });

  it('shows custom source banner only when definition.source is custom', () => {
    const { wrapper: custom } = mountDialog({ source: 'custom' });
    expect(custom.find('[data-test="custom-source-banner"]').exists()).toBe(true);
    custom.unmount();

    const { wrapper: preset } = mountDialog();
    expect(preset.find('[data-test="custom-source-banner"]').exists()).toBe(false);
  });

  it('reset-to-derived requires confirmation then emits resetToDerived', async () => {
    const { wrapper } = mountDialog({ source: 'custom' });
    await wrapper.find('[data-test="reset-to-derived"]').trigger('click');
    expect(wrapper.emitted('resetToDerived')).toBeUndefined();
    expect(dialogOnOk.current).not.toBeNull();
    dialogOnOk.current!();
    expect(wrapper.emitted('resetToDerived')).toHaveLength(1);
  });

  it('linked graph pick sets source=linked_external; clearing restores preset', async () => {
    const { wrapper, definition } = mountDialog();
    const select = wrapper.findComponent('[data-test="linked-graph-select"]');
    expect(select.exists()).toBe(true);

    select.vm.$emit('update:modelValue', 'g-9');
    await nextTick();
    expect(definition.linked_graph_id).toBe('g-9');
    expect(definition.source).toBe('linked_external');

    select.vm.$emit('update:modelValue', null);
    await nextTick();
    expect(definition.linked_graph_id).toBeUndefined();
    expect(definition.source).toBe('preset');
  });

  it('linked graph selector shows current value only for linked_external source', async () => {
    const { wrapper } = mountDialog({ source: 'linked_external', linked_graph_id: 'g-7' });
    const select = wrapper.findComponent('[data-test="linked-graph-select"]');
    expect(select.props('modelValue')).toBe('g-7');
  });

  it('checkpoint toggle mirrors definition.enable_checkpoint with default true', async () => {
    const { wrapper, definition } = mountDialog();
    const toggle = wrapper.findComponent('[data-test="checkpoint-toggle"]');
    expect(toggle.props('modelValue')).toBe(true);

    toggle.vm.$emit('update:modelValue', false);
    await nextTick();
    expect(definition.enable_checkpoint).toBe(false);
  });

  it('custom source + dirty overwrite key → save click asks confirmation before emitting save', async () => {
    // 基线指纹与当前 definition 不一致（模拟打开后又改了拓扑字段）
    const { wrapper } = mountDialog(
      { source: 'custom', mode: 'parallel' },
      { overwriteBaselineKey: 'stale-baseline' },
    );
    await wrapper.find('[data-test="team-save"]').trigger('click');
    expect(wrapper.emitted('save')).toBeUndefined();
    expect(dialogOnOk.current).not.toBeNull();
    dialogOnOk.current!();
    expect(wrapper.emitted('save')).toHaveLength(1);
  });

  it('custom source + unchanged overwrite key → save emits directly without confirmation', async () => {
    const definition = mkDefinition({ source: 'custom' });
    const baseline = `${definition.mode}|cp=x`; // 任意值——先用真实指纹函数算
    const { definitionTopologyOverwriteKey } = await import('../teamUtils');
    const { wrapper } = mountDialog({ source: 'custom' }, { overwriteBaselineKey: definitionTopologyOverwriteKey(definition) });
    void baseline;
    await wrapper.find('[data-test="team-save"]').trigger('click');
    expect(wrapper.emitted('save')).toHaveLength(1);
    expect(dialogOnOk.current).toBeNull();
  });

  it('preset source never asks overwrite confirmation even with dirty key', async () => {
    const { wrapper } = mountDialog({}, { overwriteBaselineKey: 'stale-baseline' });
    await wrapper.find('[data-test="team-save"]').trigger('click');
    expect(wrapper.emitted('save')).toHaveLength(1);
    expect(dialogOnOk.current).toBeNull();
  });
});
