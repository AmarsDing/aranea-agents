<template>
  <q-page :class="['team-orchestrate-page graph-workbench', { 'is-dark': isDark }]">
    <div class="team-orchestrate-page__toolbar graph-editor-page__toolbar">
      <q-btn flat dense round icon="arrow_back" @click="goBack" />
      <div class="graph-workbench__toolbar-meta">
        <div class="team-orchestrate-page__title">{{ teamRow?.display_name || "Team 编排" }}</div>
        <div v-if="compiled" class="graph-workbench__subtitle">
          {{ compiled.mode }} · 模板 {{ compiled.template_id }}
          <span v-if="liveMode"> · 运行中</span>
        </div>
      </div>
      <q-space />
      <q-badge v-if="liveConnected" rounded class="team-orchestrate-page__live-badge">实时</q-badge>
      <q-badge v-if="compiled" rounded :color="compiled.valid ? 'positive' : 'negative'">
        {{ compiled.valid ? "校验通过" : "校验失败" }}
      </q-badge>
      <q-btn
        v-if="liveMode && activeRun"
        flat
        dense
        icon="insights"
        label="观测台"
        color="primary"
        @click="openObservatory"
      />
      <q-btn flat dense round icon="refresh" :loading="loading" @click="reload">
        <q-tooltip>重新编译</q-tooltip>
      </q-btn>
      <q-btn flat dense round icon="save" color="primary" :loading="saving" :disable="readOnly || !dirty" @click="saveGraph">
        <q-tooltip>保存 graph 到 definition</q-tooltip>
      </q-btn>
    </div>

    <q-banner v-if="readOnly" dense rounded class="q-ma-md bg-orange-1 text-dark">
      <div class="row items-center wrap q-gutter-sm">
        <span>团队有进行中的 Run，编排定义只读。画布与看板已切换为实时模式。</span>
        <q-btn v-if="activeRun" flat dense color="primary" icon="insights" label="打开观测台" @click="openObservatory" />
      </div>
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
    <template v-else>
      <q-tabs v-model="activeTab" dense align="left" class="team-orchestrate-page__tabs" active-color="primary">
        <q-tab name="canvas" label="编排画布" />
        <q-tab name="kanban" :label="liveMode ? '工作看板' : '成员看板'" />
        <q-tab name="info" label="编排信息" />
      </q-tabs>

      <div v-show="activeTab === 'canvas'" class="team-orchestrate-page__body graph-workbench__body">
        <GraphEditorCanvas
          :graph-def="graphDef"
          :is-dark="isDark"
          :exec-node-states="execNodeStates"
          :selected-node-id="selectedNodeId"
          :focus-selected-node="liveMode"
          :read-only="readOnly"
          @select-node="onSelectNode"
          @update-graph="markDirty"
        />
        <TeamOrchestrateNodePanel
          :selected-node-id="selectedNodeId"
          :graph-def="graphDef"
          :compiled="compiled"
          :definition="definition"
          :read-only="readOnly"
          :live-state="selectedLiveState"
        />
      </div>

      <div v-show="activeTab === 'kanban'" class="team-orchestrate-page__kanban-pane">
        <OrchestrationKanban
          v-if="liveMode"
          :nodes="nodeList"
          :is-dark="isDark"
          :live-connected="liveConnected"
          :selected-node-id="selectedNodeId"
          @select-node="onKanbanSelectNode"
        />
        <TeamMemberKanban v-else :compiled="compiled" :definition="definition" :is-dark="isDark" />
      </div>

      <aside v-show="activeTab === 'info'" class="team-orchestrate-page__sidebar">
        <TeamOrchestrateRuntimePanel
          :definition="definition"
          :read-only="readOnly"
          @patch="onRuntimePatch"
        />
        <q-separator class="q-my-md" />
        <div class="text-subtitle2 q-mb-sm">编译拓扑</div>
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
              <q-item-label>{{ n.description || n.agentName || n.id }}</q-item-label>
              <q-item-label caption>{{ n.role || n.type }} · {{ n.agentName || "—" }}</q-item-label>
            </q-item-section>
          </q-item>
        </q-list>
      </aside>
    </template>
  </q-page>
</template>

<script setup lang="ts">
import { ref } from "vue";
import GraphEditorCanvas from "../components/graph/GraphEditorCanvas.vue";
import OrchestrationKanban from "../components/orchestration/OrchestrationKanban.vue";
import TeamMemberKanban from "../components/teams/TeamMemberKanban.vue";
import TeamOrchestrateRuntimePanel from "../components/teams/TeamOrchestrateRuntimePanel.vue";
import TeamOrchestrateNodePanel from "../components/teams/TeamOrchestrateNodePanel.vue";
import { useTeamOrchestratePage } from "../features/teams/useTeamOrchestratePage";

const activeTab = ref("canvas");

const {
  isDark,
  loading,
  saving,
  dirty,
  error,
  teamRow,
  compiled,
  linkedGraphId,
  definition,
  readOnly,
  selectedNodeId,
  activeRun,
  liveConnected,
  liveMode,
  nodeList,
  selectedLiveState,
  execNodeStates,
  graphDef,
  issues,
  reload,
  patchDefinition,
  markDirty,
  saveGraph,
  onSelectNode,
  openObservatory,
  goBack,
} = useTeamOrchestratePage();

function onRuntimePatch(patch: Partial<import("../features/teams/types").TeamDefinition>) {
  patchDefinition(patch);
}

function onKanbanSelectNode(nodeId: string) {
  onSelectNode(nodeId);
  activeTab.value = "canvas";
}
</script>

<style scoped>
.team-orchestrate-page__toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--glass-border);
}
.team-orchestrate-page__title {
  font-weight: 700;
  font-size: 16px;
}
.team-orchestrate-page__body {
  display: flex;
  min-height: calc(100vh - 180px);
}
.team-orchestrate-page__sidebar {
  padding: 16px;
  max-width: 420px;
}
.team-orchestrate-page__live-badge {
  background: color-mix(in srgb, var(--color-success) 18%, var(--glass-surface));
  color: var(--color-success);
}
</style>
