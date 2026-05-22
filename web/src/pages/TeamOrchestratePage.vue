<template>
  <q-page :class="['team-orchestrate-page', { 'is-dark': isDark }]">
    <div class="team-orchestrate-page__toolbar">
      <q-btn flat dense round icon="arrow_back" @click="goBack" />
      <div>
        <div class="team-orchestrate-page__title">{{ teamRow?.display_name || "Team 编排" }}</div>
        <div v-if="compiled" class="text-caption text-grey-7">
          {{ compiled.mode }} · 模板 {{ compiled.template_id }}
        </div>
      </div>
      <q-space />
      <q-badge v-if="compiled" rounded :color="compiled.valid ? 'positive' : 'negative'">
        {{ compiled.valid ? "校验通过" : "校验失败" }}
      </q-badge>
      <q-btn flat dense round icon="refresh" :loading="loading" @click="reload">
        <q-tooltip>重新编译</q-tooltip>
      </q-btn>
      <q-btn flat dense round icon="save" color="primary" :loading="saving" :disable="readOnly || !dirty" @click="saveGraph">
        <q-tooltip>保存 graph 到 definition</q-tooltip>
      </q-btn>
    </div>

    <q-banner v-if="readOnly" dense rounded class="q-ma-md bg-orange-1 text-dark">
      团队有进行中的 Run，编排定义只读。请等待 Run 结束后再编辑。
    </q-banner>

    <q-banner v-if="issues.length" dense rounded class="q-ma-md" :class="compiled?.valid ? 'bg-warning text-dark' : 'bg-red-1 text-dark'">
      <div v-for="(issue, idx) in issues" :key="idx" class="text-caption">
        {{ issue.warning ? "⚠" : "✕" }} {{ issue.message || issue.code }}
        <span v-if="issue.nodeId"> · {{ issue.nodeId }}</span>
      </div>
    </q-banner>

    <div v-if="loading" class="flex flex-center q-pa-xl">
      <q-spinner color="primary" size="40px" />
    </div>
    <div v-else-if="error" class="q-pa-md text-negative">{{ error }}</div>
    <div v-else class="team-orchestrate-page__body">
      <aside :class="['team-orchestrate-page__sidebar', { 'is-dark': isDark }]">
        <div class="text-subtitle2 q-mb-sm">编排信息</div>
        <div class="text-caption text-grey-7">入口</div>
        <div class="text-body2 q-mb-sm">{{ compiled?.entry_point || "—" }}</div>
        <div class="text-caption text-grey-7">出口</div>
        <div class="text-body2 q-mb-md">{{ compiled?.finish_point || "—" }}</div>
        <q-input
          v-model="linkedGraphId"
          dense
          outlined
          :readonly="readOnly"
          label="关联 Graph ID (linked_graph_id)"
          class="q-mb-md"
          @update:model-value="markDirty"
        />
        <div class="text-caption text-grey-7 q-mb-xs">成员节点</div>
        <q-list dense bordered separator class="rounded-borders">
          <q-item v-for="n in compiled?.nodes ?? []" :key="n.id">
            <q-item-section>
              <q-item-label>{{ n.agentName || n.id }}</q-item-label>
              <q-item-label caption>{{ n.role || n.type }}</q-item-label>
            </q-item-section>
          </q-item>
        </q-list>
      </aside>
      <GraphEditorCanvas
        :graph-def="graphDef"
        :is-dark="isDark"
        :exec-node-states="execNodeStates"
        @select-node="onSelectNode"
        @update-graph="markDirty"
      />
    </div>
  </q-page>
</template>

<script setup lang="ts">
import GraphEditorCanvas from "../components/graph/GraphEditorCanvas.vue";
import { useTeamOrchestratePage } from "../features/teams/useTeamOrchestratePage";

const {
  isDark,
  loading,
  saving,
  dirty,
  error,
  teamRow,
  compiled,
  linkedGraphId,
  readOnly,
  execNodeStates,
  graphDef,
  issues,
  markDirty,
  reload,
  saveGraph,
  onSelectNode,
  goBack
} = useTeamOrchestratePage();
</script>

<style scoped>
.team-orchestrate-page__toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--color-border-subtle, rgba(0, 0, 0, 0.08));
}
.team-orchestrate-page__title {
  font-weight: 700;
  font-size: 16px;
}
.team-orchestrate-page__body {
  display: grid;
  grid-template-columns: minmax(240px, 280px) minmax(0, 1fr);
  min-height: calc(100vh - 120px);
}
.team-orchestrate-page__sidebar {
  padding: 16px;
  border-right: 1px solid var(--color-border-subtle, rgba(0, 0, 0, 0.08));
  overflow: auto;
}
@media (max-width: 900px) {
  .team-orchestrate-page__body {
    grid-template-columns: 1fr;
  }
  .team-orchestrate-page__sidebar {
    border-right: none;
    border-bottom: 1px solid var(--color-border-subtle, rgba(0, 0, 0, 0.08));
  }
}
</style>
