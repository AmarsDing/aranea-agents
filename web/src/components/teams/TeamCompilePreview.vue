<template>
  <div :class="['team-compile-preview app-glass-side-panel', { 'is-dark': isDark }]">
    <div class="team-compile-preview__head row items-center justify-between no-wrap">
      <div class="min-width-0">
        <div class="team-compile-preview__title">编译预览</div>
        <div class="team-compile-preview__subtitle">CompileTeamGraph 实时拓扑</div>
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
      <div class="text-caption q-mb-xs">
        {{ teamModeLabel(compiled.mode) }} · 入口 {{ compiled.entry_point }} → {{ compiled.finish_point }}
      </div>
      <div class="team-compile-preview__graph">
        <div v-for="edge in compiled.edges" :key="`${edge.from}-${edge.to}`" class="team-compile-preview__edge">
          <span>{{ edge.from }}</span>
          <q-icon name="arrow_forward" size="14px" />
          <span>{{ edge.to }}</span>
          <q-badge v-if="edge.edgeKind" dense outline>{{ edge.edgeKind }}</q-badge>
        </div>
        <div class="team-compile-preview__nodes">
          <div v-for="node in compiled.nodes" :key="node.id" class="team-compile-preview__node">
            <q-icon name="smart_toy" size="16px" />
            <div class="min-width-0">
              <div class="ellipsis text-weight-medium">{{ node.description || node.agentName || node.id }}</div>
              <div class="team-compile-preview__node-meta">
                {{ node.role ? teamRoleLabel(node.role) : node.type }} · {{ node.id }}
              </div>
            </div>
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
    <div v-else class="team-compile-preview__empty">添加成员后自动编译预览</div>
  </div>
</template>

<script setup lang="ts">
import type { CompileTeamGraphResult } from '../../features/orchestration/compileApi';
import { teamModeLabel, teamRoleLabel } from './teamUtils';

defineProps<{
  isDark: boolean;
  compiled: CompileTeamGraphResult | null;
  loading: boolean;
  error: string;
  issues: Array<{ message?: string; code?: string; warning?: boolean }>;
}>();
</script>
