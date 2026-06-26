<!--
  Team 域展示组件：仅 props / emits（aranea-frontend-guide SKILL §1 红线 #1）。
  路径约定：SKILL §3.3 → `web/src/components/teams/`。
-->
<template>
  <q-dialog :model-value="modelValue" position="right" @update:model-value="$emit('update:modelValue', $event)">
    <q-card :class="['team-runs-panel app-dialog-card app-glass-dialog', { 'is-dark': isDark }]">
      <q-card-section class="row items-center justify-between">
        <div>
          <div class="text-h6">运行轨迹</div>
          <div class="row items-center q-gutter-sm text-caption text-grey-7">
            <span>{{ selectedTeam?.display_name || 'Team' }} · 最近 30 次</span>
            <q-badge rounded :color="liveConnected ? 'positive' : 'grey'">{{
              liveConnected ? '实时连接中' : '未连接'
            }}</q-badge>
          </div>
        </div>
        <q-btn flat round icon="close" @click="$emit('update:modelValue', false)" />
      </q-card-section>
      <q-separator />
      <q-card-section>
        <q-btn flat rounded color="primary" icon="refresh" label="刷新" :loading="loading" @click="$emit('refresh')" />
        <q-banner v-if="liveReplaying" dense rounded class="bg-blue-1 text-dark q-mt-sm">
          <template #avatar><q-spinner-dots color="primary" size="18px" /></template>
          正在同步历史 WS 事件…
        </q-banner>
        <div v-if="error" class="text-negative q-mt-sm">{{ error }}</div>
        <q-list v-else class="q-mt-md run-list" separator>
          <q-expansion-item v-for="run in runs" :key="run.id" group="team-runs" @show="$emit('showSteps', run.id)">
            <template #header>
              <q-item-section avatar>
                <q-avatar :color="runStatusColor(run.status)" text-color="white" :icon="runStatusIcon(run.status)" />
              </q-item-section>
              <q-item-section>
                <q-item-label>{{ run.mode }} · {{ run.duration_ms }}ms</q-item-label>
                <q-item-label caption
                  >{{ formatDate(run.created_at) }} · in {{ run.token_in }} / out {{ run.token_out }} ·
                  {{ formatCost(run.cost_micro_usd) }}</q-item-label
                >
              </q-item-section>
            </template>
            <div class="run-detail q-pa-md">
              <div class="text-caption text-grey-7 q-mb-sm">输入</div>
              <div class="run-preview">{{ run.input_preview || '-' }}</div>
              <div class="text-caption text-grey-7 q-mt-md q-mb-sm">输出</div>
              <div class="run-preview">{{ run.output_preview || run.error_message || '-' }}</div>
              <q-separator class="q-my-md" />
              <div class="row items-center q-gutter-sm q-mb-sm">
                <q-btn
                  flat
                  dense
                  size="sm"
                  color="primary"
                  icon="summarize"
                  label="加载汇总"
                  :loading="Boolean(summariesLoading[run.id])"
                  @click="$emit('loadSummary', run.id)"
                />
                <q-btn
                  flat
                  dense
                  size="sm"
                  color="primary"
                  icon="insights"
                  label="观测台"
                  @click="$emit('openObservatory', run.id)"
                />
              </div>
              <div v-if="summariesByRun[run.id]" class="run-summary q-mb-md">
                <div class="text-caption text-grey-7">结构化汇总</div>
                <div class="text-body2">
                  成员 {{ summariesByRun[run.id]?.member_count }} · 工具调用
                  {{ summariesByRun[run.id]?.tool_call_count }} · in {{ summariesByRun[run.id]?.token_in }} / out
                  {{ summariesByRun[run.id]?.token_out }}
                </div>
              </div>
              <q-inner-loading :showing="Boolean(stepsLoading[run.id])" />
              <div v-for="step in stepsByRun[run.id] || []" :key="step.id" class="run-step">
                <div class="row items-center justify-between">
                  <div class="text-weight-medium">
                    {{ step.agent_name || step.agent_key || agentName(agents, step.agent_id) }}
                  </div>
                  <q-badge rounded :color="stepStatusColor(step.status)">{{ step.status }}</q-badge>
                </div>
                <div class="text-caption text-grey-7">
                  {{ step.role || 'worker' }} · {{ step.duration_ms }}ms · in {{ step.token_in }} / out
                  {{ step.token_out }}
                  <span v-if="step.tool_call_count"> · 工具 {{ step.tool_call_count }}</span>
                  · {{ formatCost(step.cost_micro_usd) }}
                </div>
                <div class="run-preview q-mt-sm">{{ step.output_preview || step.error_message || '-' }}</div>
              </div>
              <div v-if="!stepsLoading[run.id] && !(stepsByRun[run.id] || []).length" class="text-caption text-grey-7">
                暂无步骤记录。
              </div>
            </div>
          </q-expansion-item>
        </q-list>
        <div v-if="!loading && runs.length === 0 && !error" class="text-center text-grey-7 q-pa-xl">
          暂无运行记录，请先进入 Chat 测试该 Team。
        </div>
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import type { Agent } from '../../features/agents/types';
import type { Team, TeamRun, TeamRunStep, TeamRunSummary } from '../../features/teams/types';
import { agentName, formatDate } from './teamUtils';

defineProps<{
  modelValue: boolean;
  selectedTeam: Team | null;
  runs: TeamRun[];
  stepsByRun: Record<string, TeamRunStep[]>;
  stepsLoading: Record<string, boolean>;
  summariesByRun: Record<string, TeamRunSummary>;
  summariesLoading: Record<string, boolean>;
  agents: Agent[];
  loading: boolean;
  error: string;
  liveConnected: boolean;
  liveReplaying?: boolean;
  isDark: boolean;
}>();

defineEmits<{
  'update:modelValue': [value: boolean];
  refresh: [];
  showSteps: [runID: string];
  loadSummary: [runID: string];
  openObservatory: [runID: string];
}>();

function formatCost(value?: number) {
  return `$${((value ?? 0) / 1_000_000).toFixed(4)}`;
}

function stepStatusColor(status: string) {
  const normalized = status.trim().toLowerCase();
  if (normalized === 'running' || normalized === 'pending') return 'info';
  if (normalized === 'success' || normalized === 'ok' || normalized === 'completed') return 'positive';
  if (normalized === 'failed' || normalized === 'error') return 'negative';
  if (normalized === 'cancelled' || normalized === 'canceled') return 'warning';
  return 'grey';
}

function runStatusColor(status: string) {
  return stepStatusColor(status);
}

function runStatusIcon(status: string) {
  const normalized = status.trim().toLowerCase();
  if (normalized === 'running' || normalized === 'pending') return 'sync';
  if (normalized === 'success' || normalized === 'ok' || normalized === 'completed') return 'check';
  if (normalized === 'failed' || normalized === 'error') return 'error';
  if (normalized === 'cancelled' || normalized === 'canceled') return 'cancel';
  return 'schedule';
}
</script>
