<template>
  <div :class="['team-compile-preview app-glass-side-panel', { 'is-dark': isDark }]">
    <div class="team-compile-preview__head row items-center justify-between no-wrap">
      <div class="min-width-0">
        <div class="team-compile-preview__title">编排流程预览</div>
        <div class="team-compile-preview__subtitle">根据成员与模式实时生成</div>
      </div>
      <q-badge
        v-if="compiled"
        rounded
        class="team-compile-preview__badge"
        :class="compiled.valid ? 'is-valid' : 'is-invalid'"
      >
        {{ compiled.valid ? '通过' : '失败' }}
      </q-badge>
    </div>

    <div v-if="loading" class="flex flex-center q-pa-lg">
      <q-spinner color="primary" size="28px" />
    </div>
    <div v-else-if="error" class="text-caption text-negative q-pa-sm">{{ error }}</div>
    <template v-else-if="compiled">
      <div class="text-body2 q-mb-md">{{ topologySummary }}</div>
      <div class="team-compile-preview__nodes">
        <div v-for="row in memberRows" :key="row.key" class="team-compile-preview__node">
          <q-icon name="smart_toy" size="16px" />
          <div class="min-width-0">
            <div class="ellipsis text-weight-medium">{{ row.name }}</div>
            <div class="team-compile-preview__node-meta">{{ row.roleLabel }}</div>
          </div>
        </div>
      </div>
      <div v-if="issues.length" class="q-mt-sm">
        <div
          v-for="(issue, idx) in issues"
          :key="idx"
          class="text-caption"
          :class="issue.warning ? 'text-warning' : 'text-negative'"
        >
          {{ issue.warning ? '⚠' : '✕' }} {{ issue.message || issue.code }}
        </div>
      </div>
    </template>
    <div v-else class="team-compile-preview__empty">添加成员后自动生成流程预览</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { CompileTeamGraphResult } from '../../features/orchestration/compileApi';
import { teamMemberDisplayRows, teamTopologySummary } from '../../features/orchestration/teamNodeDisplay';
import type { TeamDefinition } from '../../features/teams/types';

const props = defineProps<{
  isDark: boolean;
  compiled: CompileTeamGraphResult | null;
  definition: TeamDefinition | null;
  loading: boolean;
  error: string;
  issues: Array<{ message?: string; code?: string; warning?: boolean }>;
}>();

const topologySummary = computed(() => teamTopologySummary(props.compiled, props.definition));
const memberRows = computed(() => teamMemberDisplayRows(props.compiled, props.definition));
</script>
