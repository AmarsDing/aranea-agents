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
        <OrchestrationKanban
          :nodes="nodeList"
          :is-dark="isDark"
          :live-connected="streamConnected"
          :selected-node-id="selectedNodeId"
          @select-node="onSelectNode"
        />
      </section>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import GraphEditorCanvas from "../components/graph/GraphEditorCanvas.vue";
import OrchestrationKanban from "../components/orchestration/OrchestrationKanban.vue";
import { useTeamRunObservatoryPage } from "../features/teams/useTeamRunObservatoryPage";

const {
  isDark,
  runId,
  loading,
  error,
  observatory,
  selectedNodeId,
  graphDef,
  streamConnected,
  nodeList,
  execNodeStates,
  runStatusColor,
  onSelectNode,
  goBack
} = useTeamRunObservatoryPage();
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
