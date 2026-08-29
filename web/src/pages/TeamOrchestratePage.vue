<template>
  <q-page :class="['team-orchestrate-page graph-workbench', { 'is-dark': isDark }]">
    <div class="team-orchestrate-page__toolbar graph-editor-page__toolbar">
      <q-btn flat dense round icon="arrow_back" @click="goBack" />
      <div class="graph-workbench__toolbar-meta">
        <div class="team-orchestrate-page__title">{{ teamRow?.display_name || 'Team 编排' }}</div>
        <div v-if="compiled" class="graph-workbench__subtitle">
          {{ teamModeLabel(compiled.mode) }}编排
          <span v-if="liveMode"> · 运行中</span>
        </div>
      </div>
      <q-space />
      <q-badge v-if="liveConnected" rounded class="team-orchestrate-page__live-badge">实时</q-badge>
      <AppStatusChip v-if="compiled" :status="compiled.valid ? 'valid' : 'invalid'" />
      <q-btn
        v-if="liveMode && activeRun"
        flat
        dense
        icon="insights"
        label="观测台"
        color="primary"
        @click="openObservatory"
      />
      <q-btn
        flat
        dense
        icon="account_tree"
        label="在 Graph 编辑器中打开"
        :loading="openingGraphEditor"
        data-test="open-graph-editor"
        @click="openInGraphEditor"
      />
      <q-btn flat dense round icon="refresh" :loading="loading" @click="reload">
        <q-tooltip>重新编译</q-tooltip>
      </q-btn>
    </div>

    <q-banner v-if="readOnly" dense rounded class="q-ma-md bg-orange-1 text-dark">
      <div class="row items-center wrap q-gutter-sm">
        <span>团队有进行中的 Run，画布与看板已切换为实时模式。编排编辑请在团队编辑对话框中进行。</span>
        <q-btn
          v-if="activeRun"
          flat
          dense
          color="primary"
          icon="insights"
          label="打开观测台"
          @click="openObservatory"
        />
      </div>
    </q-banner>

    <q-banner
      v-if="issues.length"
      dense
      rounded
      class="q-ma-md"
      :class="compiled?.valid ? 'bg-warning text-dark' : 'bg-red-1 text-dark'"
    >
      <button
        v-for="(issue, idx) in issues"
        :key="idx"
        type="button"
        class="team-orchestrate-page__issue text-caption"
        :class="{ 'team-orchestrate-page__issue--link': Boolean(issue.nodeId) }"
        :disabled="!issue.nodeId"
        :data-test="issue.nodeId ? 'issue-node-link' : undefined"
        @click="onIssueClick(issue.nodeId)"
      >
        {{ issue.warning ? '⚠' : '✕' }} {{ issue.message || issue.code }}
        <span v-if="issue.nodeId"> · {{ issue.nodeId }}</span>
      </button>
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
          v-model:graph-def="graphDef"
          :is-dark="isDark"
          :exec-node-states="execNodeStates"
          :selected-node-id="selectedNodeId"
          :focus-selected-node="liveMode"
          :read-only="true"
          :hide-tech-ids="true"
          :node-issues="canvasNodeIssues"
          @select-node="onSelectNode"
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
        <div class="text-subtitle2 q-mb-sm">执行方式</div>
        <div class="text-body2 q-mb-md">{{ topologySummary || '—' }}</div>

        <div class="text-subtitle2 q-mb-sm">成员</div>
        <q-list v-if="memberRows.length" dense bordered separator class="rounded-borders q-mb-md">
          <q-item v-for="row in memberRows" :key="row.key">
            <q-item-section>
              <q-item-label>{{ row.name }}</q-item-label>
              <q-item-label caption>{{ row.roleLabel }}</q-item-label>
            </q-item-section>
          </q-item>
        </q-list>
        <div v-else class="text-caption app-text-secondary q-mb-md">{{ t('teamsPage.orchestrateNoMembers') }}</div>

        <q-separator class="q-my-md" />
        <TeamOrchestrateRuntimePanel :definition="definition" />
      </aside>
    </template>
  </q-page>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import GraphEditorCanvas from '../components/graph/GraphEditorCanvas.vue';
import OrchestrationKanban from '../components/orchestration/OrchestrationKanban.vue';
import TeamMemberKanban from '../components/teams/TeamMemberKanban.vue';
import TeamOrchestrateRuntimePanel from '../components/teams/TeamOrchestrateRuntimePanel.vue';
import TeamOrchestrateNodePanel from '../components/teams/TeamOrchestrateNodePanel.vue';
import { teamModeLabel } from '../components/teams/teamConstants';
import {
  teamMemberDisplayRows,
  teamTopologySummary,
  compileIssuesToNodeIssues,
} from '../features/orchestration/teamNodeDisplay';
import { useTeamOrchestratePage } from '../features/teams/useTeamOrchestratePage';

const activeTab = ref('canvas');
const { t } = useI18n();

const {
  isDark,
  loading,
  error,
  teamRow,
  compiled,
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
  onSelectNode,
  openObservatory,
  openingGraphEditor,
  openInGraphEditor,
  goBack,
} = useTeamOrchestratePage();

// M53 Phase 11 F3：校验问题 → 画布节点错误态（同节点 error 优先于 warning）。
const canvasNodeIssues = computed(() => compileIssuesToNodeIssues(issues.value));

/** 校验问题联动：点击带 nodeId 的 issue → 选中节点并切回画布。 */
function onIssueClick(nodeId?: string) {
  const id = String(nodeId ?? '').trim();
  if (!id) return;
  onSelectNode(id);
  activeTab.value = 'canvas';
}

const topologySummary = computed(() => teamTopologySummary(compiled.value, definition.value));
const memberRows = computed(() => teamMemberDisplayRows(compiled.value, definition.value));

function onKanbanSelectNode(nodeId: string) {
  onSelectNode(nodeId);
  activeTab.value = 'canvas';
}
</script>
