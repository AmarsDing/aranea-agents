<template>
  <q-page class="app-standard-page cap-page memory-page">
    <!-- 看谁的记忆：Agent 选择器放在页签行右边；Agent 多，支持输入过滤 -->
    <Teleport defer to="#app-tab-actions">
      <q-select
        :model-value="selectedAgentId"
        class="mem-agent-pick"
        dense
        outlined
        clearable
        emit-value
        map-options
        use-input
        hide-selected
        fill-input
        input-debounce="0"
        options-dense
        :placeholder="selectedAgentId ? '' : t('cap.memory.pickAgent')"
        :aria-label="t('cap.memory.pickAgent')"
        :options="agentPickOptions"
        @filter="onAgentFilter"
        @update:model-value="selectedAgentId = $event as string | null"
      >
        <template #prepend><q-icon name="smart_toy" size="18px" /></template>
        <template #no-option>
          <q-item dense>
            <q-item-section class="text-caption">{{ t('cap.memory.noAgentMatch') }}</q-item-section>
          </q-item>
        </template>
      </q-select>
      <q-btn
        flat
        dense
        round
        icon="refresh"
        :title="t('cap.common.refresh')"
        :aria-label="t('cap.common.refresh')"
        :loading="loading"
        @click="loadAll"
      />
    </Teleport>

    <AppStatePanel
      v-if="pageState === 'error'"
      variant="error"
      :description="error"
      :action-label="t('memory.error.retry')"
      action-icon="refresh"
      @action="loadAll"
    />
    <AppStatePanel v-else-if="pageState === 'forbidden'" variant="forbidden" />
    <AppStatePanel v-else-if="pageState === 'loading'" variant="loading" :rows="5" />

    <template v-else>
      <AppStatePanel
        v-if="pageState === 'partial'"
        variant="partial"
        dense
        :description="error"
        :action-label="t('memory.error.retry')"
        action-icon="refresh"
        @action="loadAll"
      />

      <div class="mem-shell">
        <!-- 左侧：七个视图，每个写一句它回答什么问题；治理有待处理时带数 -->
        <nav class="mem-rail" :aria-label="t('cap.memory.views')">
          <button
            v-for="item in railItems"
            :key="item.name"
            type="button"
            class="mem-rail__item"
            :class="{ 'mem-rail__item--active': tab === item.name }"
            :aria-current="tab === item.name ? 'page' : undefined"
            @click="tab = item.name"
          >
            <q-icon :name="item.icon" size="18px" aria-hidden="true" />
            <span class="mem-rail__text">
              <span class="mem-rail__label">{{ item.label }}</span>
              <span class="mem-rail__desc">{{ item.desc }}</span>
            </span>
            <span v-if="item.count" class="mem-rail__count">{{ item.count }}</span>
          </button>

          <!-- 平台视角：哪些 Agent 有记忆（点一下切过去）；不属于任何 Agent 的按作用域列出 -->
          <section
            v-if="footprint.agents.length || footprint.unowned.length"
            class="mem-foot"
            :aria-label="t('cap.memory.footprintTitle')"
          >
            <h3 class="mem-foot__title">{{ t('cap.memory.footprintTitle') }}</h3>
            <button
              v-for="fp in footprint.agents.slice(0, 6)"
              :key="fp.agent_id"
              type="button"
              class="mem-foot__row"
              :aria-pressed="selectedAgentId === fp.agent_id"
              @click="selectedAgentId = fp.agent_id"
            >
              <span class="mem-foot__name">{{ agentName(fp.agent_id) }}</span>
              <span class="cap-mono">{{ fp.facts + fp.episodes }}</span>
            </button>
            <p v-if="!footprint.agents.length" class="mem-foot__empty">{{ t('cap.memory.footprintNone') }}</p>
            <p v-for="u in footprint.unowned" :key="u.scope_type" class="mem-foot__unowned">
              {{ t('cap.memory.unowned', { scope: u.scope_type, n: u.facts }) }}
            </p>
          </section>
        </nav>

        <div class="mem-main">
          <p v-if="!selectedAgentId && agentScopedTab" class="cap-note cap-note--info" role="note">
            <q-icon name="info" size="16px" aria-hidden="true" />
            <span class="cap-note__body">{{ t('cap.memory.agentFirst') }}</span>
          </p>

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
                  <memory-episode-timeline
                    v-if="show('L2')"
                    :agent-id="selectedAgentId"
                    :session-id="selectedSessionId"
                  />
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

            <q-tab-panel name="notes">
              <memory-notes-tab />
            </q-tab-panel>

            <!-- 治理：四件事分开看，一次只看一件，各带待处理数 -->
            <q-tab-panel name="governance">
              <div class="mem-gov-bar">
                <div class="cap-seg" role="group" :aria-label="t('cap.memory.rail.governance')">
                  <button
                    v-for="seg in governanceSegments"
                    :key="seg.value"
                    type="button"
                    :aria-pressed="governance === seg.value"
                    @click="governance = seg.value"
                  >
                    {{ seg.label }}<span v-if="seg.count" class="mem-seg-count">{{ seg.count }}</span>
                  </button>
                </div>
              </div>
              <memory-conflict-panel
                v-if="governance === 'conflict'"
                :rows="conflictFacts"
                :loading="loadingConflicts"
                :acting-id="conflictActingId"
                @refresh="loadFacts"
                @open-fact="openFact"
                @review="reviewConflictFact"
              />
              <memory-cascade-panel
                v-else-if="governance === 'cascade'"
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
              <div v-else-if="governance === 'evolution'" class="column q-gutter-md">
                <memory-evolution-review-panel
                  :proposals="evolutionProposals"
                  :events="evolutionEvents"
                  :acting-id="evolutionActingId"
                  @approve="reviewEvolutionProposal($event, 'approve')"
                  @reject="reviewEvolutionProposal($event, 'reject')"
                  @revert="revertEvolutionEvent"
                />
                <memory-evolution-panel :panels="evolutionPanels" />
              </div>
              <MemoryPIIPanel
                v-else
                :rows="piiFacts"
                :loading="loadingPII"
                :acting-id="piiActingId"
                @refresh="loadPIIFacts"
                @open-fact="openFact"
                @review="reviewPIIFactRow"
              />
            </q-tab-panel>

            <q-tab-panel name="pending">
              <memory-fact-pending-panel :agent-id="selectedAgentId" />
            </q-tab-panel>

            <!-- 运维（平台管理员）：设置与后台状态并排，死信和召回测试在下面 -->
            <q-tab-panel v-if="isPlatformAdmin" name="ops">
              <div class="mem-ops-grid">
                <memory-platform-settings-panel />
                <memory-worker-status-panel
                  :status="workerStatus"
                  :loading="loadingWorkerStatus"
                  @refresh="loadWorkerStatus"
                />
                <memory-dead-letter-panel
                  ref="deadLetterPanelRef"
                  class="mem-ops-grid__wide"
                  @replay="onDeadLetterReplay"
                  @abandon="onDeadLetterAbandon"
                />
                <memory-recall-tester-panel
                  class="mem-ops-grid__wide"
                  :agent-id="selectedAgentId"
                  :session-id="selectedSessionId"
                />
              </div>
            </q-tab-panel>
          </q-tab-panels>
        </div>
      </div>
    </template>

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
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import { resolvePageState } from './pageState';
import MemoryDeadLetterPanel from '../features/memory/MemoryDeadLetterPanel.vue';
import MemoryPlatformSettingsPanel from '../features/memory/MemoryPlatformSettingsPanel.vue';
import MemoryRecallTesterPanel from '../features/memory/MemoryRecallTesterPanel.vue';
import MemoryWorkerStatusPanel from '../components/memory/MemoryWorkerStatusPanel.vue';
import MemoryBrowseTab, { type BrowseLayer } from '../features/memory/browse/MemoryBrowseTab.vue';
import MemoryEpisodeTimeline from '../features/memory/browse/MemoryEpisodeTimeline.vue';
import MemoryNotesTab from '../features/memory/notes/MemoryNotesTab.vue';
import MemoryCascadePanel from '../features/memory/MemoryCascadePanel.vue';
import MemoryConflictPanel from '../features/memory/MemoryConflictPanel.vue';
import MemoryEvolutionPanel from '../components/memory/MemoryEvolutionPanel.vue';
import MemoryEvolutionReviewPanel from '../features/memory/MemoryEvolutionReviewPanel.vue';
import MemoryPIIPanel from '../features/memory/MemoryPIIPanel.vue';
import MemoryFactDrawer from '../features/memory/MemoryFactDrawer.vue';
import MemoryFactPendingPanel from '../features/memory/MemoryFactPendingPanel.vue';
import MemoryFactEditDialog from '../components/memory/MemoryFactEditDialog.vue';
import MemoryKnowledgePanel from '../features/memory/MemoryKnowledgePanel.vue';
import MemoryPanoramaTab from '../features/memory/panorama/MemoryPanoramaTab.vue';
import MemorySagaDrawer from '../features/memory/MemorySagaDrawer.vue';
import MemorySessionsPanel from '../features/memory/MemorySessionsPanel.vue';
import MemorySnapshotDrawer from '../features/memory/MemorySnapshotDrawer.vue';
import UnifiedMemoryGraph from '../features/memory/graph/UnifiedMemoryGraph.vue';
import type { UnifiedGraphNode } from '../features/memory/types';
import { useMemoryCenterPage } from '../features/memory/useMemoryCenterPage';
import { annotateAgentOptions } from '../features/memory/memoryFootprint';

const deadLetterPanelRef = ref<InstanceType<typeof MemoryDeadLetterPanel> | null>(null);
const { t } = useI18n();

/** 浏览 Tab 层级过滤（chips 状态由 MemoryBrowseTab 编辑，钻取跳转由本页写入）。 */
const browseLayer = ref<BrowseLayer>('all');

/** 治理视图里当前看哪一件事 */
const governance = ref<'conflict' | 'cascade' | 'evolution' | 'pii'>('conflict');

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
  forbidden,
  loading,
  loadingFacts,
  loadingSessions,
  loadingSnapshots,
  loadingTasks,
  agentOptions,
  footprint,
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

// ——— Agent 选择：输入即过滤（Agent 有几百个） ———
const agentNeedle = ref('');
// 有记忆的 Agent 排到前面并标上条数
const annotatedAgentOptions = computed(() =>
  annotateAgentOptions(agentOptions.value, footprint.value, (n) => t('cap.memory.itemCount', { n })),
);
const agentPickOptions = computed(() => {
  const kw = agentNeedle.value.trim().toLowerCase();
  const all = annotatedAgentOptions.value;
  return kw ? all.filter((o) => o.label.toLowerCase().includes(kw)) : all;
});
function agentName(id: string): string {
  return agentOptions.value.find((o) => o.value === id)?.label ?? id;
}
function onAgentFilter(val: string, update: (fn: () => void) => void) {
  update(() => {
    agentNeedle.value = val;
  });
}

// 这几个视图按 Agent 取数：没选 Agent 时提示先选
const agentScopedTab = computed(() => ['panorama', 'graph', 'browse', 'pending'].includes(tab.value));

const governanceSegments = computed(() => [
  { value: 'conflict' as const, label: t('cap.memory.gov.conflict'), count: conflictFacts.value.length },
  { value: 'cascade' as const, label: t('cap.memory.gov.cascade'), count: cascadeProposals.value.length },
  { value: 'evolution' as const, label: t('cap.memory.gov.evolution'), count: evolutionProposals.value.length },
  { value: 'pii' as const, label: t('cap.memory.gov.pii'), count: piiFacts.value.length },
]);

const railItems = computed(() => {
  const items = [
    {
      name: 'panorama',
      icon: 'dashboard',
      label: t('memory.tabs.panorama'),
      desc: t('cap.memory.rail.panorama'),
      count: 0,
    },
    { name: 'graph', icon: 'bubble_chart', label: t('memory.tabs.graph'), desc: t('cap.memory.rail.graph'), count: 0 },
    {
      name: 'browse',
      icon: 'travel_explore',
      label: t('memory.tabs.browse'),
      desc: t('cap.memory.rail.browse'),
      count: 0,
    },
    { name: 'notes', icon: 'sticky_note_2', label: t('memory.tabs.notes'), desc: t('cap.memory.rail.notes'), count: 0 },
    {
      name: 'governance',
      icon: 'verified_user',
      label: t('memory.tabs.governance'),
      desc: t('cap.memory.rail.governance'),
      count: governanceSegments.value.reduce((n, s) => n + s.count, 0),
    },
    {
      name: 'pending',
      icon: 'pending_actions',
      label: t('memory.tabs.pending'),
      desc: t('cap.memory.rail.pending'),
      count: 0,
    },
  ];
  if (isPlatformAdmin.value) {
    items.push({
      name: 'ops',
      icon: 'admin_panel_settings',
      label: t('memory.tabs.ops'),
      desc: t('cap.memory.rail.ops'),
      count: 0,
    });
  }
  return items;
});

/**
 * 五态判定：记忆中心是多子面板容器，`count` 取首屏两个主列表（会话/事实）的行数，
 * 用于区分「首屏全空 → loading/error/空态占位」与「已有数据 → partial 横幅不遮面板」。
 */
const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: sessionRows.value.length + factRows.value.length,
    forbidden: forbidden.value,
  }),
);

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
  border-radius: 12px;
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
  border-radius: 12px;
  color: var(--color-text-heading);
  line-height: 1.55;
  margin: var(--space-3) 0 0;
  max-height: 320px;
  overflow: auto;
  padding: var(--space-3);
  white-space: pre-wrap;
}

.memory-drawer {
  background: var(--surface-base);
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
