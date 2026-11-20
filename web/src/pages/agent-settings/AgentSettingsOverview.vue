<template>
  <!-- 装配图（04-Harness系统.md §6）：一眼看出这个 Agent 装了什么，点块直达对应分区编辑 -->
  <div class="as-ov">
    <button
      v-for="b in blocks"
      :key="b.key"
      type="button"
      class="as-blk"
      :class="[`as-blk--${b.key}`, { 'as-blk--guard': b.key === 'guard' }]"
      @click="$emit('go', b.tab)"
    >
      <span class="as-blk__head">
        <q-icon :name="b.icon" size="18px" aria-hidden="true" />
        <span class="as-blk__t">{{ t(`agentAssembly.block.${b.key}`) }}</span>
        <span class="as-blk__edit">{{ t('agentAssembly.edit') }} <q-icon name="arrow_forward" size="14px" /></span>
      </span>
      <span class="as-blk__hint">{{ t(`agentAssembly.hint.${b.key}`) }}</span>
      <dl class="as-blk__facts">
        <div v-for="f in b.facts" :key="f.label">
          <dt>{{ f.label }}</dt>
          <dd :class="{ 'as-mono': f.mono, 'as-bad': f.bad, 'as-dim': f.dim }">{{ f.value }}</dd>
        </div>
      </dl>
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Agent } from '../../features/agents/types';
import type { AgentAdvancedSettingsForm, AgentRuntimeConfigForm } from '../../features/agents/agentRuntimeConfig';
import { PLANNER_KIND_OPTIONS, type PlannerFormState } from '../../features/agents/plannerConfig';
import { formatContext, promptModeLabel } from '../../components/agents/agentUi';

const props = defineProps<{
  form: Agent;
  config: AgentRuntimeConfigForm;
  plannerForm: PlannerFormState;
  advanced: AgentAdvancedSettingsForm;
  orgPath: string;
  roleLabel: string;
  fileCount: number;
  fileTokens: number;
  modelOk: boolean | null;
  channelName: string;
}>();

defineEmits<{ go: [tab: string] }>();

const { t } = useI18n();

type Fact = { label: string; value: string; mono?: boolean; bad?: boolean; dim?: boolean };

const onOff = (v: boolean) => (v ? t('agentAssembly.on') : t('agentAssembly.off'));

const blocks = computed(() => {
  const c = props.config;
  const f = props.form;
  const layers = [c.memoryL1.enabled, c.memoryL2.episode_enabled, c.memoryL3.enabled, c.memoryL4.enabled].filter(
    Boolean,
  ).length;
  const planner = PLANNER_KIND_OPTIONS.find((o) => o.value === props.plannerForm.kind)?.label ?? props.plannerForm.kind;
  const budget =
    f.budget_monthly_cents > 0 ? `$${(f.budget_monthly_cents / 100).toFixed(0)}` : t('agentAssembly.budgetNone');
  const out: Array<{ key: string; tab: string; icon: string; facts: Fact[] }> = [
    {
      key: 'identity',
      tab: 'agent',
      icon: 'badge',
      facts: [
        { label: t('agentAssembly.org'), value: props.orgPath || t('agentAssembly.unseated'), dim: !props.orgPath },
        { label: t('agentRoster.col.role'), value: props.roleLabel },
        { label: t('agentAssembly.promptMode'), value: promptModeLabel(f.system_prompt_mode) },
        {
          label: t('agentAssembly.files'),
          value: t('agentAssembly.filesValue', { n: props.fileCount, tokens: props.fileTokens.toLocaleString() }),
        },
        { label: t('agentAssembly.planner'), value: planner },
      ],
    },
    {
      key: 'brain',
      tab: 'agent',
      icon: 'memory',
      facts: [
        { label: t('agentAssembly.model'), value: `${f.provider || '—'} / ${f.model || '—'}`, mono: true },
        {
          label: t('agentRoster.col.status'),
          value:
            props.modelOk === null
              ? t('agentAssembly.modelUnchecked')
              : props.modelOk
                ? t('agentAssembly.modelOk')
                : t('agentAssembly.modelBad'),
          bad: props.modelOk === false,
          dim: props.modelOk === null,
        },
        {
          label: t('agentAssembly.reasoning'),
          value: `${props.advanced.reasoning_mode} · ${props.advanced.reasoning_level}`,
          mono: true,
        },
        { label: t('agentAssembly.context'), value: formatContext(f.context_window) },
      ],
    },
    {
      key: 'hands',
      tab: 'skills',
      icon: 'build',
      facts: [
        {
          label: t('agentAssembly.tools'),
          value: c.tools.enabled
            ? t('agentAssembly.toolsValue', {
                profile: c.tools.profile,
                allow: c.tools.allow.length,
                deny: c.tools.deny.length,
              })
            : t('agentAssembly.toolsOff'),
          dim: !c.tools.enabled,
        },
        {
          label: t('agentAssembly.subagents'),
          value: c.subagents.enabled
            ? t('agentAssembly.subagentsValue', { n: c.subagents.max_concurrency })
            : onOff(false),
          dim: !c.subagents.enabled,
        },
        { label: t('agentAssembly.skills'), value: c.skill_load_mode, mono: true },
        { label: t('agentAssembly.intent'), value: onOff(c.skillRuntime.intent_routing_enabled) },
      ],
    },
    {
      key: 'memory',
      tab: 'memory',
      icon: 'psychology',
      facts: [
        { label: t('agentAssembly.memoryMain'), value: onOff(c.memory.enabled), dim: !c.memory.enabled },
        { label: t('agentAssembly.layers'), value: t('agentAssembly.layersValue', { n: layers }) },
      ],
    },
    {
      key: 'growth',
      tab: 'evolution',
      icon: 'auto_awesome',
      facts: [
        { label: t('agentAssembly.styleEvolve'), value: onOff(Boolean(c.evolution.self_evolve)) },
        { label: t('agentAssembly.pipeline'), value: onOff(c.evolutionSettings.enabled) },
        {
          label: t('agentAssembly.pending'),
          value: t('agentAssembly.pendingValue', { n: f.pending_evolution_count ?? 0 }),
          dim: !f.pending_evolution_count,
        },
      ],
    },
    {
      key: 'reach',
      tab: 'a2a',
      icon: 'hub',
      facts: [
        {
          label: t('agentAssembly.a2a'),
          value: f.agent_kind === 'a2a_proxy' ? t('agentAssembly.a2aProxy') : t('agentAssembly.a2aEndpoint'),
        },
        {
          label: t('agentAssembly.channel'),
          value: props.channelName || t('agentAssembly.channelNone'),
          dim: !props.channelName,
        },
      ],
    },
    {
      key: 'guard',
      tab: 'permissions',
      icon: 'shield',
      facts: [
        { label: t('agentAssembly.budget'), value: budget, mono: true },
        { label: t('agentAssembly.hooks'), value: t('agentAssembly.hooksValue'), dim: true },
      ],
    },
  ];
  return out;
});
</script>
