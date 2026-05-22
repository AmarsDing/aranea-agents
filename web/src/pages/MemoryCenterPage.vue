<template>
  <q-page class="memory-page">
    <div class="app-page-shell">
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
        <q-tab name="sessions" icon="account_tree" label="会话记忆" />
        <q-tab v-if="showGraphTab" name="evolution" icon="auto_awesome" label="图谱与进化" />
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

      <q-tab-panel v-if="showGraphTab" name="evolution">
        <memory-evolution-panel :panels="evolutionPanels" />
      </q-tab-panel>

      <q-tab-panel name="settings">
        <memory-settings-status-panel :items="settingChecklist" />
      </q-tab-panel>
    </q-tab-panels>

    <memory-snapshot-drawer v-model="snapshotDrawer" :snapshot="selectedSnapshot" />
    <memory-fact-drawer v-model="factDrawer" :fact="selectedFact" />
    </div>
  </q-page>
</template>

<script setup lang="ts">
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
  showGraphTab,
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
  resetFactFilters,
  openSnapshot,
  openFact
} = useMemoryCenterPage();
</script>

<style>
.memory-page {
  min-height: 100%;
  background:
    radial-gradient(circle at 8% 0%, rgb(25 118 210 / 12%), transparent 30%),
    radial-gradient(circle at 92% 12%, rgb(156 39 176 / 8%), transparent 28%),
    linear-gradient(180deg, var(--color-page-tint) 0%, var(--color-page-tint-alt) 48%, var(--color-on-accent) 100%);
}

.memory-hero {
  align-items: center;
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 18px 4px 20px;
}

.memory-kicker {
  color: var(--color-info);
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.memory-title {
  color: var(--color-text-heading);
  font-size: clamp(30px, 4vw, 46px);
  font-weight: 900;
  letter-spacing: -0.04em;
  line-height: 1.05;
  margin: 4px 0;
}

.memory-subtitle {
  color: var(--color-text-tertiary);
  margin: 0;
  max-width: 760px;
}

.memory-select {
  min-width: 220px;
}

.memory-card,
.memory-tabs-card {
  border-radius: 18px;
  border: 1px solid var(--glass-border);
  background: var(--glass-surface);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
}

.memory-tabs-card {
  margin-bottom: 12px;
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
  gap: 12px;
}

.memory-flow-node {
  align-items: center;
  border: 1px solid rgb(15 23 42 / 8%);
  border-radius: 16px;
  display: flex;
  gap: 12px;
  padding: 14px;
}

.memory-active-item {
  background: rgb(25 118 210 / 8%);
  color: var(--q-primary);
}

.memory-info-banner {
  background: var(--color-info-soft);
  color: var(--color-status-info-text-light);
}

.memory-pre {
  background: var(--color-surface-soft);
  border: 1px solid rgb(15 23 42 / 8%);
  border-radius: 14px;
  color: var(--color-text-heading);
  line-height: 1.55;
  margin: 12px 0 0;
  max-height: 320px;
  overflow: auto;
  padding: 12px;
  white-space: pre-wrap;
}

.memory-drawer {
  background: var(--color-on-accent);
}

body.body--dark .memory-page {
  background: var(--canvas-base);
}

body.body--dark .memory-title {
  color: var(--color-text-primary);
}

body.body--dark .memory-subtitle,
body.body--dark .memory-page .text-grey-7 {
  color: var(--color-text-secondary) !important;
}

body.body--dark .memory-card,
body.body--dark .memory-tabs-card {
  background: var(--glass-surface);
  border-color: var(--glass-border);
  box-shadow: none;
}

body.body--dark .memory-flow-node {
  border-color: rgb(148 163 184 / 18%);
  background: rgb(15 23 42 / 50%);
}

body.body--dark .memory-info-banner {
  background: rgb(30 64 175 / 24%);
  color: var(--color-accent-blue-light);
}

body.body--dark .memory-pre {
  background: var(--color-surface-solid);
  border-color: rgb(148 163 184 / 20%);
  color: var(--color-text-dark);
}

body.body--dark .memory-drawer {
  background: var(--color-surface-elevated);
}

@media (width <= 800px) {
  .memory-hero {
    align-items: stretch;
    flex-direction: column;
  }

  .memory-select {
    min-width: 100%;
  }
}
</style>
