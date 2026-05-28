<template>
  <q-page class="app-standard-page memory-page">
    <memory-hero v-model:selected-agent-id="selectedAgentId" :agent-options="agentOptions" :loading="loading" @refresh="loadAll" />

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadAll" />
      </template>
    </q-banner>

    <memory-metric-cards :cards="overviewCards" />

    <q-card flat class="memory-tabs-card">
      <q-tabs v-model="tab" align="left" active-color="primary" indicator-color="primary" no-caps outside-arrows mobile-arrows>
        <q-tab name="overview" icon="hub" label="总览" />
        <q-tab name="knowledge" icon="psychology" label="知识库" />
        <q-tab name="cascade" icon="sync_alt" label="Cascade" />
        <q-tab name="sessions" icon="account_tree" label="会话记忆" />
        <q-tab name="evolution" icon="auto_awesome" label="图谱与进化" />
        <q-tab name="settings" icon="tune" label="设置" />
      </q-tabs>
    </q-card>

    <q-tab-panels v-model="tab" animated class="memory-panels">
      <q-tab-panel name="overview">
        <memory-overview-panel :memory-layers="memoryLayers" :action-items="actionItems" />
      </q-tab-panel>

      <q-tab-panel name="knowledge">
        <memory-knowledge-panel
          v-model:fact-keyword="factKeyword"
          v-model:fact-scope="factScope"
          v-model:fact-status="factStatus"
          :facts-endpoint-ready="factsEndpointReady"
          :scope-options="scopeOptions"
          :fact-status-options="factStatusOptions"
          :fact-rows="factRows"
          :fact-columns="factColumns"
          :loading-facts="loadingFacts"
          @reset="resetFactFilters"
          @search="loadFacts"
          @open-fact="openFact"
        />
      </q-tab-panel>

      <q-tab-panel name="cascade">
        <memory-cascade-panel
          :agent-id="selectedAgentId"
          :rows="cascadeProposals"
          :loading="loadingCascade"
          :acting-id="cascadeActingId"
          @refresh="loadCascade"
          @approve="approveCascade"
          @reject="rejectCascade"
        />
      </q-tab-panel>

      <q-tab-panel name="sessions">
        <memory-sessions-panel
          v-model:selected-session-id="selectedSessionId"
          :session-rows="sessionRows"
          :loading-sessions="loadingSessions"
          :snapshot-rows="snapshotRows"
          :snapshot-columns="snapshotColumns"
          :loading-snapshots="loadingSnapshots"
          :task-rows="taskRows"
          @refresh-sessions="loadSessions"
          @refresh-memory="loadSessionMemory"
          @open-snapshot="openSnapshot"
        />
      </q-tab-panel>

      <q-tab-panel name="evolution">
        <memory-graph-explorer :entities="entities" :loading-entities="loadingEvolution" @refresh="loadEvolution" />
        <memory-evolution-panel class="q-mt-md" :panels="evolutionPanels" />
      </q-tab-panel>

      <q-tab-panel name="settings">
        <div class="column q-gutter-md">
          <memory-platform-settings-panel />
          <memory-worker-status-panel />
          <memory-recall-tester-panel :agent-id="selectedAgentId" :session-id="selectedSessionId" />
          <memory-settings-status-panel :items="settingChecklist" />
        </div>
      </q-tab-panel>
    </q-tab-panels>

    <memory-snapshot-drawer v-model="snapshotDrawer" :snapshot="selectedSnapshot" />
    <memory-fact-drawer v-model="factDrawer" :fact="selectedFact" />
  </q-page>
</template>

<script setup lang="ts">
import MemoryPlatformSettingsPanel from "../features/memory/MemoryPlatformSettingsPanel.vue";
import MemoryGraphExplorer from "../features/memory/MemoryGraphExplorer.vue";
import MemoryRecallTesterPanel from "../features/memory/MemoryRecallTesterPanel.vue";
import MemoryWorkerStatusPanel from "../features/memory/MemoryWorkerStatusPanel.vue";
import MemoryCascadePanel from "../features/memory/MemoryCascadePanel.vue";
import MemoryEvolutionPanel from "../features/memory/MemoryEvolutionPanel.vue";
import MemoryFactDrawer from "../features/memory/MemoryFactDrawer.vue";
import MemoryHero from "../features/memory/MemoryHero.vue";
import MemoryKnowledgePanel from "../features/memory/MemoryKnowledgePanel.vue";
import MemoryMetricCards from "../features/memory/MemoryMetricCards.vue";
import MemoryOverviewPanel from "../features/memory/MemoryOverviewPanel.vue";
import MemorySessionsPanel from "../features/memory/MemorySessionsPanel.vue";
import MemorySettingsStatusPanel from "../features/memory/MemorySettingsStatusPanel.vue";
import MemorySnapshotDrawer from "../features/memory/MemorySnapshotDrawer.vue";
import { useMemoryCenterPage } from "../features/memory/useMemoryCenterPage";

const {
  tab,
  selectedAgentId,
  selectedSessionId,
  selectedSnapshot,
  selectedFact,
  factKeyword,
  factScope,
  factStatus,
  snapshotDrawer,
  factDrawer,
  error,
  loading,
  loadingFacts,
  loadingSessions,
  loadingSnapshots,
  agentOptions,
  sessionRows,
  factRows,
  snapshotRows,
  taskRows,
  overviewCards,
  actionItems,
  memoryLayers,
  evolutionPanels,
  entities,
  loadingEvolution,
  cascadeProposals,
  loadingCascade,
  cascadeActingId,
  settingChecklist,
  scopeOptions,
  factStatusOptions,
  factColumns,
  snapshotColumns,
  factsEndpointReady,
  loadAll,
  loadSessions,
  loadFacts,
  loadSessionMemory,
  loadCascade,
  loadEvolution,
  approveCascade,
  rejectCascade,
  resetFactFilters,
  openSnapshot,
  openFact
} = useMemoryCenterPage();
</script>

<style>
.memory-card,
.memory-tabs-card {
  border-radius: 18px;
  border: 1px solid var(--glass-border);
  background: var(--glass-surface);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
}

.memory-tabs-card {
  margin-bottom: var(--space-3);
  overflow: hidden;
}

.memory-panels {
  background: transparent;
}

.memory-panels .q-tab-panel {
  padding: 0;
}

.memory-flow {
  display: grid;
  gap: var(--space-3);
}

.memory-flow-node {
  align-items: center;
  border: 1px solid var(--glass-border);
  border-radius: 16px;
  display: flex;
  gap: var(--space-3);
  padding: var(--space-3);
}

.memory-active-item {
  background: color-mix(in srgb, var(--color-accent) 8%, transparent);
  color: var(--q-primary);
}

.memory-info-banner {
  background: var(--color-info-soft);
  color: var(--color-status-info-text-light);
}

.memory-pre {
  background: var(--color-surface-soft);
  border: 1px solid var(--glass-border);
  border-radius: 14px;
  color: var(--color-text-heading);
  line-height: 1.55;
  margin: var(--space-3) 0 0;
  max-height: 320px;
  overflow: auto;
  padding: var(--space-3);
  white-space: pre-wrap;
}

.memory-drawer {
  background: var(--color-on-accent);
}

body.body--dark .memory-card,
body.body--dark .memory-tabs-card {
  background: var(--glass-surface);
  border-color: var(--glass-border);
  box-shadow: none;
}

body.body--dark .memory-flow-node {
  border-color: var(--glass-border-hover);
  background: var(--glass-surface);
}

body.body--dark .memory-info-banner {
  background: color-mix(in srgb, var(--color-accent) 24%, transparent);
  color: var(--color-accent-blue-light);
}

body.body--dark .memory-pre {
  background: var(--color-surface-solid);
  border-color: var(--glass-border-hover);
  color: var(--color-text-dark);
}

body.body--dark .memory-drawer {
  background: var(--color-surface-elevated);
}

body.body--dark .memory-page .text-grey-7 {
  color: var(--color-text-secondary);
}
</style>
