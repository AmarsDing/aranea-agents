<template>
  <q-page :class="['team-run-observatory', { 'is-dark': isDark }]">
    <div class="team-run-observatory__toolbar">
      <q-btn flat dense round icon="arrow_back" @click="goBack" />
      <div>
        <div class="team-run-observatory__title">Team 运行观测</div>
        <div v-if="observatory" class="text-caption text-grey-7">
          {{ observatory.mode }} · {{ observatory.status }} · {{ runId }}
        </div>
      </div>
      <q-space />
      <q-badge v-if="observatory" rounded :color="runStatusColor">{{ observatory.status }}</q-badge>
    </div>

    <div v-if="loading" class="flex flex-center q-pa-xl">
      <q-spinner color="primary" size="40px" />
    </div>
    <div v-else-if="error" class="q-pa-md text-negative">{{ error }}</div>
    <div v-else class="team-run-observatory__body">
      <section class="team-run-observatory__graph">
        <GraphEditorCanvas
          :graph-def="graphDef"
          :is-dark="isDark"
          :exec-node-states="execNodeStates"
          :selected-node-id="selectedNodeId"
          :focus-selected-node="true"
          @select-node="onSelectNode"
          @update-graph="() => {}"
        />
      </section>
      <section class="team-run-observatory__kanban">
        <q-tabs v-model="observatoryTab" dense align="left" class="q-mb-md">
          <q-tab name="agents" label="Agent 工作看板" />
          <q-tab name="tasks" label="任务看板" :disable="!graphExecutionId" />
        </q-tabs>
        <q-tab-panels v-model="observatoryTab" animated>
          <q-tab-panel name="agents" class="q-pa-none">
            <OrchestrationKanban
              :nodes="nodeList"
              :is-dark="isDark"
              :live-connected="streamConnected"
              :selected-node-id="selectedNodeId"
              @select-node="onKanbanSelectNode"
            />
          </q-tab-panel>
          <q-tab-panel name="tasks" class="q-pa-none">
            <GraphTaskKanban
              v-if="graphExecutionId"
              :tasks="taskList"
              :loading="tasksLoading"
              :live-connected="taskStreamConnected"
              :selected-task-id="selectedTaskId"
              :is-dark="isDark"
              admin-drag
              @refresh="loadObservatoryTasks"
              @select-task="onSelectTask"
              @admin-action="onKanbanAdminAction"
            />
            <div v-else class="text-caption text-grey-7 q-pa-sm">
              当前 Team Run 未绑定 Graph 执行，任务看板不可用。
            </div>
          </q-tab-panel>
        </q-tab-panels>
      </section>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import GraphEditorCanvas from "../components/graph/GraphEditorCanvas.vue";
import GraphTaskKanban from "../components/graph/GraphTaskKanban.vue";
import OrchestrationKanban from "../components/orchestration/OrchestrationKanban.vue";
import { useTeamRunObservatoryPage } from "../features/teams/useTeamRunObservatoryPage";

const {
  isDark,
  runId,
  loading,
  error,
  observatory,
  observatoryTab,
  selectedNodeId,
  selectedTaskId,
  graphDef,
  streamConnected,
  taskStreamConnected,
  nodeList,
  taskList,
  tasksLoading,
  graphExecutionId,
  execNodeStates,
  runStatusColor,
  onSelectNode,
  onSelectTask,
  onKanbanAdminAction,
  loadObservatoryTasks,
  goBack,
} = useTeamRunObservatoryPage();

function onKanbanSelectNode(nodeId: string) {
  onSelectNode(nodeId);
}
</script>

<style scoped>
.team-run-observatory__toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--color-border-subtle, rgba(0, 0, 0, 0.08));
}
.team-run-observatory__title {
  font-weight: 700;
  font-size: 16px;
}
.team-run-observatory__body {
  display: grid;
  grid-template-columns: minmax(0, 1.1fr) minmax(0, 0.9fr);
  gap: 0;
  min-height: calc(100vh - 120px);
}
.team-run-observatory__graph {
  min-height: 420px;
  border-right: 1px solid var(--color-border-subtle, rgba(0, 0, 0, 0.08));
}
.team-run-observatory__kanban {
  padding: 16px;
  overflow: auto;
}
@media (max-width: 1024px) {
  .team-run-observatory__body {
    grid-template-columns: 1fr;
  }
  .team-run-observatory__graph {
    border-right: none;
    border-bottom: 1px solid var(--color-border-subtle, rgba(0, 0, 0, 0.08));
  }
}
</style>
