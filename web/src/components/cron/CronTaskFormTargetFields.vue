<!--
  Cron 表单：目标类型（纯展示子块，v-model:form + props）。
-->
<template>
  <div class="app-grid-span-full">
    <div class="section-label q-mb-sm">目标类型</div>
    <q-btn-toggle
      v-model="form.target_type"
      spread
      no-caps
      unelevated
      toggle-color="primary"
      class="cron-btn-toggle"
      :options="cronTargetToggleOptions"
    />
  </div>

  <q-select
    v-if="form.target_type === 'agent'"
    v-model="form.agent_id"
    class="cron-field app-grid-span-full app-field-md"
    dense
    outlined
    clearable
    emit-value
    map-options
    label="Agent"
    hint="留空时调度器使用默认 Agent"
    :options="agentOptions"
  />
  <q-select
    v-else
    v-model="form.team_id"
    class="cron-field app-grid-span-full app-field-md"
    dense
    outlined
    emit-value
    map-options
    label="Team *"
    :options="teamOptions"
    :rules="[cronTeamRule]"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { Agent } from '../../features/agents/types';
import type { CronTaskFormValue } from '../../features/cron/types';
import type { Team } from '../../features/teams/types';
import { cronAgentSelectOptions, cronTargetToggleOptions, cronTeamRule, cronTeamSelectOptions } from './cronTaskUtils';

const form = defineModel<CronTaskFormValue>('form', { required: true });

const props = defineProps<{
  agents: Agent[];
  teams: Team[];
}>();

const agentOptions = computed(() => cronAgentSelectOptions(props.agents));
const teamOptions = computed(() => cronTeamSelectOptions(props.teams));
</script>
