<template>
  <q-page class="app-standard-page memory-page">
    <memory-hero
      v-model:selected-agent-id="selectedAgentId"
      :agent-options="agentOptions"
      :loading="loading"
      @refresh="loadAll"
    />

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" :label="t('memory.error.retry')" @click="loadAll" />
      </template>
    </q-banner>

    <q-card flat class="memory-tabs-card">
      <q-tabs
        v-model="tab"
        align="left"
        active-color="primary"
        indicator-color="primary"
        no-caps
        outside-arrows
        mobile-arrows
      >
        <q-tab name="panorama" icon="dashboard" :label="t('memory.tabs.panorama')" />
        <q-tab name="graph" icon="bubble_chart" :label="t('memory.tabs.graph')" />
        <q-tab name="browse" icon="travel_explore" :label="t('memory.tabs.browse')" />
        <q-tab name="governance" icon="verified_user" :label="t('memory.tabs.governance')" />
        <q-tab v-if="isPlatformAdmin" name="ops" icon="admin_panel_settings" :label="t('memory.tabs.ops')" />
      </q-tabs>
    </q-card>

    <q-tab-panels v-model="tab" animated class="memory-panels">
      <q-tab-panel name="panorama">
        <memory-panorama-tab
          :agent-id="selectedAgentId"
          :session-id="selectedSessionId"
          @drill-layer="onDrillLayer"
          @navigate-tab="onNavigateTab"
        />
      </q-tab-panel>

      <q-tab-panel name="graph">
        <unified-memory-graph :agent-id="selectedAgentId" @open-in-browse="onOpenInBrowse" />
      </q-tab-panel>

      <q-tab-panel name="browse">
        <memory-browse-tab v-model:layer="browseLayer">
          <template #default="{ show }">
            <memory-sessions-panel
              v-if="show('L0') || show('L1')"
              v-model:selected-session-id="selectedSessionId"
              v-model:selected-task-id="selectedTaskId"
              :session-rows="sessionRows"
              :loading-sessions="loadingSessions"
              :snapshot-rows="snapshotRows"
              :snapshot-columns="snapshotColumns"
              :loading-snapshots="loadingSnapshots"
              :task-rows="taskRows"
              :loading-tasks="loadingTasks"
              :field-rows="fieldRows"
              :loading-fields="loadingFields"
              @refresh-sessions="loadSessions"
              @refresh-memory="loadSessionMemory"
              @open-snapshot="openSnapshot"
            />
            <memory-episode-timeline v-if="show('L2')" :agent-id="selectedAgentId" :session-id="selectedSessionId" />
            <memory-knowledge-panel
              v-if="show('L3')"
              v-model:fact-keyword="factKeyword"
              v-model:fact-scope="factScope"
              v-model:fact-status="factStatus"
              v-model:page="factPage"
              v-model:page-size="factPageSize"
              :facts-endpoint-ready="factsEndpointReady"
              :scope-options="scopeOptions"
              :fact-status-options="factStatusOptions"
              :fact-rows="factRows"
              :fact-columns="factColumns"
              :loading-facts="loadingFacts"
              :facts-active-count="factsActiveCount"
              :facts-archived-count="factsArchivedCount"
              :facts-total="factsFilteredCount"
              :page-max="factPageMax"
              @reset="resetFactFilters"
              @search="reloadFactsFromFirstPage"
              @open-fact="openFact"
              @create-fact="openCreateFact"
            />
          </template>
        </memory-browse-tab>
      </q-tab-panel>

      <q-tab-panel name="governance">
        <div class="column q-gutter-md">
          <memory-conflict-panel
            :rows="conflictFacts"
            :loading="loadingConflicts"
            :acting-id="conflictActingId"
            @refresh="loadFacts"
            @open-fact="openFact"
            @review="reviewConflictFact"
          />
          <memory-cascade-panel
            v-model:preview-open="cascadePreviewOpen"
            :agent-id="selectedAgentId"
            :rows="cascadeProposals"
            :loading="loadingCascade"
            :acting-id="cascadeActingId"
            :preview-loading="loadingCascadePreview"
            :preview="cascadePreviewData"
            :preview-proposal-id="cascadePreviewProposalId"
            @refresh="loadCascade"
            @approve="approveCascade"
            @reject="rejectCascade"
            @preview="previewCascade"
            @confirm-preview="confirmPreviewCascade"
            @saga="openSagaDrawer"
            @retry="retryCascade"
            @compensate="compensateCascade"
          />
          <memory-evolution-panel :panels="evolutionPanels" />
          <memory-evolution-review-panel
            :proposals="evolutionProposals"
            :events="evolutionEvents"
            :acting-id="evolutionActingId"
            @approve="reviewEvolutionProposal($event, 'approve')"
            @reject="reviewEvolutionProposal($event, 'reject')"
            @revert="revertEvolutionEvent"
          />
          <MemoryPIIPanel
            :rows="piiFacts"
            :loading="loadingPII"
            :acting-id="piiActingId"
            @refresh="loadPIIFacts"
            @open-fact="openFact"
            @review="reviewPIIFactRow"
          />
        </div>
      </q-tab-panel>

      <q-tab-panel v-if="isPlatformAdmin" name="ops">
        <div class="column q-gutter-md">
          <memory-platform-settings-panel />
          <memory-worker-status-panel
            :status="workerStatus"
            :loading="loadingWorkerStatus"
            @refresh="loadWorkerStatus"
          />
          <memory-dead-letter-panel
            ref="deadLetterPanelRef"
            @replay="onDeadLetterReplay"
            @abandon="onDeadLetterAbandon"
          />
          <memory-recall-tester-panel :agent-id="selectedAgentId" :session-id="selectedSessionId" />
        </div>
      </q-tab-panel>
    </q-tab-panels>

    <memory-snapshot-drawer v-model="snapshotDrawer" :snapshot="selectedSnapshot" />
    <memory-fact-drawer
      v-model="factDrawer"
      :fact="selectedFact"
      :acting="factReviewActing"
      @review="reviewSelectedFact"
      @refine="openRefineFact"
    />
    <memory-fact-edit-dialog
      v-model:open="factEditOpen"
      :mode="factEditMode"
      :fact="selectedFact"
      :saving="factReviewActing"
      @submit="submitFactEdit"
    />
    <memory-saga-drawer v-model="cascadeSagaDrawerOpen" :loading="loadingCascadeSaga" :steps="sagaSteps" />
  </q-page>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import MemoryDeadLetterPanel from '../features/memory/MemoryDeadLetterPanel.vue';
import MemoryPlatformSettingsPanel from '../features/memory/MemoryPlatformSettingsPanel.vue';
import MemoryRecallTesterPanel from '../features/memory/MemoryRecallTesterPanel.vue';
import MemoryWorkerStatusPanel from '../components/memory/MemoryWorkerStatusPanel.vue';
import MemoryBrowseTab, { type BrowseLayer } from '../features/memory/browse/MemoryBrowseTab.vue';
import MemoryEpisodeTimeline from '../features/memory/browse/MemoryEpisodeTimeline.vue';
import MemoryCascadePanel from '../features/memory/MemoryCascadePanel.vue';
import MemoryConflictPanel from '../features/memory/MemoryConflictPanel.vue';
import MemoryEvolutionPanel from '../components/memory/MemoryEvolutionPanel.vue';
import MemoryEvolutionReviewPanel from '../features/memory/MemoryEvolutionReviewPanel.vue';
import MemoryPIIPanel from '../features/memory/MemoryPIIPanel.vue';
import MemoryFactDrawer from '../features/memory/MemoryFactDrawer.vue';
import MemoryFactEditDialog from '../components/memory/MemoryFactEditDialog.vue';
import MemoryHero from '../components/memory/MemoryHero.vue';
import MemoryKnowledgePanel from '../features/memory/MemoryKnowledgePanel.vue';
import MemoryPanoramaTab from '../features/memory/panorama/MemoryPanoramaTab.vue';
import MemorySagaDrawer from '../features/memory/MemorySagaDrawer.vue';
import MemorySessionsPanel from '../features/memory/MemorySessionsPanel.vue';
import MemorySnapshotDrawer from '../features/memory/MemorySnapshotDrawer.vue';
import UnifiedMemoryGraph from '../features/memory/graph/UnifiedMemoryGraph.vue';
import type { UnifiedGraphNode } from '../features/memory/types';
import { useMemoryCenterPage } from '../features/memory/useMemoryCenterPage';

const deadLetterPanelRef = ref<InstanceType<typeof MemoryDeadLetterPanel> | null>(null);
const { t } = useI18n();

/** 浏览 Tab 层级过滤（chips 状态由 MemoryBrowseTab 编辑，钻取跳转由本页写入）。 */
const browseLayer = ref<BrowseLayer>('all');

const {
  tab,
  isPlatformAdmin,
  selectMemoryTab,
  selectedAgentId,
  selectedSessionId,
  selectedSnapshot,
  selectedFact,
  factKeyword,
  factScope,
  factStatus,
  snapshotDrawer,
  factDrawer,
  factEditOpen,
  factEditMode,
  factReviewActing,
  conflictFacts,
  loadingConflicts,
  conflictActingId,
  selectedTaskId,
  fieldRows,
  loadingFields,
  piiFacts,
  loadingPII,
  piiActingId,
  evolutionActingId,
  evolutionProposals,
  evolutionEvents,
  loadPIIFacts,
  reviewPIIFactRow,
  reviewEvolutionProposal,
  revertEvolutionEvent,
  error,
  loading,
  loadingFacts,
  loadingSessions,
  loadingSnapshots,
  loadingTasks,
  agentOptions,
  sessionRows,
  factRows,
  snapshotRows,
  taskRows,
  deepLinkLayer,
  evolutionPanels,
  cascadeProposals,
  loadingCascade,
  cascadeActingId,
  cascadePreviewOpen,
  cascadePreviewData,
  cascadePreviewProposalId,
  loadingCascadePreview,
  cascadeSagaDrawerOpen,
  sagaSteps,
  loadingCascadeSaga,
  scopeOptions,
  factStatusOptions,
  factColumns,
  snapshotColumns,
  factsEndpointReady,
  factsActiveCount,
  factsArchivedCount,
  factsFilteredCount,
  factPage,
  factPageSize,
  factPageMax,
  loadAll,
  loadSessions,
  loadFacts,
  reloadFactsFromFirstPage,
  loadSessionMemory,
  loadCascade,
  approveCascade,
  rejectCascade,
  previewCascade,
  openSagaDrawer,
  retryCascade,
  compensateCascade,
  confirmPreviewCascade,
  resetFactFilters,
  openSnapshot,
  openFact,
  reviewSelectedFact,
  openRefineFact,
  openCreateFact,
  submitFactEdit,
  reviewConflictFact,
  handleDeadLetterReplay,
  handleDeadLetterAbandon,
  workerStatus,
  loadingWorkerStatus,
  loadWorkerStatus,
} = useMemoryCenterPage();

watch(
  deepLinkLayer,
  (layer) => {
    if (layer === 'L0' || layer === 'L1' || layer === 'L2' || layer === 'L3') {
      browseLayer.value = layer;
    }
  },
  { immediate: true },
);

async function onDeadLetterReplay(id: number) {
  await handleDeadLetterReplay(id);
  deadLetterPanelRef.value?.load();
}

async function onDeadLetterAbandon(id: number) {
  await handleDeadLetterAbandon(id);
  deadLetterPanelRef.value?.load();
}

// 层级卡钻取（终态矩阵 §10.6.2）：L0/L1/L2/L3 → browse + layer；L4 → graph。
function onDrillLayer(layer: string) {
  if (layer === 'L4') {
    tab.value = 'graph';
    return;
  }
  if (layer === 'L0' || layer === 'L1' || layer === 'L2' || layer === 'L3') {
    browseLayer.value = layer;
    tab.value = 'browse';
    return;
  }
  tab.value = 'panorama';
}

// 需要关注跳转：target_tab 已是终态命名（browse/governance/panorama），直达。
function onNavigateTab(target: string) {
  selectMemoryTab(target);
}

// 图谱节点「在记忆浏览中打开」（FR-R8 终态矩阵）：事实 → browse L3 按完整 statement 过滤（label 已截断，需从 meta 取全文）；情景 → browse L2；实体 → graph 聚焦。
async function onOpenInBrowse(node: UnifiedGraphNode) {
  if (node.kind === 'fact') {
    factKeyword.value = parseNodeMetaStatement(node) || node.label;
    browseLayer.value = 'L3';
    tab.value = 'browse';
    await reloadFactsFromFirstPage();
    return;
  }
  if (node.kind === 'episode') {
    browseLayer.value = 'L2';
    tab.value = 'browse';
    return;
  }
  tab.value = 'graph';
}

/** 从节点 meta_json 解析完整事实文本（label 被截断为 40 字符，不能直接用于搜索）。 */
function parseNodeMetaStatement(node: UnifiedGraphNode): string {
  try {
    const meta = JSON.parse(node.meta_json || '{}') as { statement?: unknown };
    return typeof meta.statement === 'string' ? meta.statement : '';
  } catch {
    return '';
  }
}
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
