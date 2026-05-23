<template>
  <div :class="['team-compile-preview', { 'is-dark': isDark }]">
    <div class="team-compile-preview__head row items-center justify-between">
      <div>
        <div class="text-subtitle2">编译预览</div>
        <div class="text-caption app-text-secondary">CompileTeamGraph 实时拓扑（后端真相源）</div>
      </div>
      <q-badge v-if="compiled" rounded :color="compiled.valid ? 'positive' : 'negative'">
        {{ compiled.valid ? "通过" : "失败" }}
      </q-badge>
    </div>

    <div v-if="loading" class="flex flex-center q-pa-lg">
      <q-spinner color="primary" size="28px" />
    </div>
    <div v-else-if="error" class="text-caption text-negative q-pa-sm">{{ error }}</div>
    <template v-else-if="compiled">
      <div class="text-caption q-mb-xs">
        {{ compiled.mode }} · entry {{ compiled.entry_point }} → {{ compiled.finish_point }}
      </div>
      <div class="team-compile-preview__graph">
        <div v-for="edge in compiled.edges" :key="`${edge.from}-${edge.to}`" class="team-compile-preview__edge">
          <span>{{ edge.from }}</span>
          <q-icon name="arrow_forward" size="14px" />
          <span>{{ edge.to }}</span>
          <q-badge v-if="edge.kind" dense outline>{{ edge.kind }}</q-badge>
        </div>
        <div class="team-compile-preview__nodes">
          <div v-for="node in compiled.nodes" :key="node.id" class="team-compile-preview__node">
            <q-icon name="smart_toy" size="16px" />
            <div class="min-width-0">
              <div class="ellipsis text-weight-medium">{{ node.description || node.agentName || node.id }}</div>
              <div class="text-caption text-grey-7">{{ node.role || node.type }} · {{ node.id }}</div>
            </div>
          </div>
        </div>
      </div>
      <div v-if="issues.length" class="q-mt-sm">
        <div v-for="(issue, idx) in issues" :key="idx" class="text-caption" :class="issue.warning ? 'text-warning' : 'text-negative'">
          {{ issue.warning ? "⚠" : "✕" }} {{ issue.message || issue.code }}
        </div>
      </div>
    </template>
    <div v-else class="text-caption text-grey-7 q-pa-sm">添加成员后自动编译预览</div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from "vue";
import { compileTeamGraph, type CompileTeamGraphResult } from "../../features/orchestration/compileApi";

const props = defineProps<{
  teamId: string;
  definitionJson: string;
  isDark: boolean;
}>();

const loading = ref(false);
const error = ref("");
const compiled = ref<CompileTeamGraphResult | null>(null);
const issues = ref<Array<{ message?: string; code?: string; warning?: boolean }>>([]);

let debounceTimer: ReturnType<typeof setTimeout> | null = null;

async function refresh() {
  const json = props.definitionJson?.trim();
  if (!json || json === "{}" || !json.includes("members")) {
    compiled.value = null;
    return;
  }
  loading.value = true;
  error.value = "";
  try {
    const teamId = props.teamId?.trim() || "draft-preview";
    compiled.value = await compileTeamGraph(teamId, json);
    issues.value = (compiled.value.issues ?? []).map((i) => ({
      message: i.message,
      code: i.code,
      warning: Boolean(i.warning),
    }));
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
    compiled.value = null;
  } finally {
    loading.value = false;
  }
}

function scheduleRefresh() {
  if (debounceTimer) clearTimeout(debounceTimer);
  debounceTimer = setTimeout(refresh, 400);
}

watch(() => [props.teamId, props.definitionJson], scheduleRefresh, { immediate: true });
</script>

<style scoped>
.team-compile-preview {
  border: 1px solid var(--glass-border);
  border-radius: 16px;
  padding: 12px;
  background: color-mix(in srgb, var(--glass-surface) 88%, transparent);
  min-height: 280px;
}

.team-compile-preview__graph {
  display: grid;
  gap: 8px;
  max-height: 360px;
  overflow: auto;
}

.team-compile-preview__edge {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  font-size: 11px;
  color: var(--color-text-secondary);
}

.team-compile-preview__nodes {
  display: grid;
  gap: 6px;
}

.team-compile-preview__node {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 12px;
  border: 1px solid var(--glass-border);
  background: var(--glass-elevated);
}

.min-width-0 {
  min-width: 0;
}
</style>
