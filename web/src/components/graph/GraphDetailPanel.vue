<template>
  <aside :class="['graph-detail-panel', { 'is-dark': isDark }]">
    <template v-if="graph">
      <div class="graph-detail-panel__header">
        <div class="graph-detail-panel__title" :title="graph.name">{{ graph.name }}</div>
        <span class="graph-detail-panel__version">v{{ graph.version }}</span>
        <span :class="['graph-detail-panel__status', `graph-detail-panel__status--${status}`]">
          <i class="graph-detail-panel__status-dot" />{{ t(GRAPH_CARD_STATUS_LABEL_KEYS[status]) }}
        </span>
        <q-btn flat dense round icon="close" size="sm" @click="emit('close')" />
      </div>

      <!-- R2-B.1 紧凑操作条：30px 单行 5 操作 -->
      <div class="graph-detail-panel__actionbar">
        <q-btn flat dense no-caps icon="edit" data-action="edit" :label="t('graphs.detailActionEdit')" @click="emit('edit', graph.id)" />
        <q-btn flat dense no-caps icon="play_arrow" data-action="run" :label="t('graphs.detailActionRun')" @click="emit('run', graph)" />
        <q-btn flat dense no-caps icon="content_copy" data-action="duplicate" :label="t('graphs.detailActionDuplicate')" @click="emit('duplicate', graph)" />
        <q-btn flat dense no-caps icon="download" data-action="export" :label="t('graphs.detailActionExport')" @click="emit('export', graph)" />
        <q-btn flat dense no-caps icon="delete" data-action="delete" color="negative" :label="t('graphs.detailActionDelete')" @click="emit('delete', graph)" />
      </div>

      <!-- R2-B.2 统计行 -->
      <div class="graph-detail-panel__stats-row">
        <div class="graph-detail-panel__stat-cell">
          <div class="graph-detail-panel__stat-num">{{ graph.nodes?.length ?? 0 }}</div>
          <div class="graph-detail-panel__stat-label">{{ t('graphs.detailStatNodes') }}</div>
        </div>
        <div class="graph-detail-panel__stat-cell">
          <div class="graph-detail-panel__stat-num">{{ (graph.edges?.length ?? 0) + (graph.conditionalEdges?.length ?? 0) }}</div>
          <div class="graph-detail-panel__stat-label">{{ t('graphs.detailStatEdges') }}</div>
        </div>
        <div class="graph-detail-panel__stat-cell">
          <div class="graph-detail-panel__stat-num">{{ graph.stateFields?.length ?? 0 }}</div>
          <div class="graph-detail-panel__stat-label">{{ t('graphs.detailStatFields') }}</div>
        </div>
        <div class="graph-detail-panel__stat-cell">
          <div class="graph-detail-panel__stat-num">{{ execCountLabel }}</div>
          <div class="graph-detail-panel__stat-label">{{ t('graphs.detailStatRuns') }}</div>
        </div>
      </div>

      <div class="graph-detail-panel__body">
        <!-- 基本信息（紧凑，引擎只读 + tooltip 说明） -->
        <div class="graph-detail-panel__info">
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">{{ t('graphs.detailLabelEngine') }}</span>
            <span class="graph-detail-panel__value">
              {{ graph.executionEngine === 'dag' ? t('graphs.engineDAG') : t('graphs.engineBSP') }}
              <q-tooltip>{{
                graph.executionEngine === 'dag' ? t('graphs.engineDAGHint') : t('graphs.engineBSPHint')
              }}</q-tooltip>
            </span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">{{ t('graphs.detailLabelCheckpoint') }}</span>
            <span class="graph-detail-panel__value">{{
              graph.enableCheckpoint ? t('graphs.detailCheckpointEnabled') : t('graphs.detailCheckpointDisabled')
            }}</span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">{{ t('graphs.detailLabelEntry') }}</span>
            <span class="graph-detail-panel__value graph-detail-panel__value--mono">{{ graph.entryPoint || '—' }}</span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">{{ t('graphs.detailLabelFinish') }}</span>
            <span class="graph-detail-panel__value graph-detail-panel__value--mono">{{ graph.finishPoint || '—' }}</span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">{{ t('graphs.detailLabelUpdatedAt') }}</span>
            <span class="graph-detail-panel__value">{{ relativeTime(graph.updatedAt) }}</span>
          </div>
        </div>

        <!-- R2-B.3/B.4/B.5/B.7 节点 section（可折叠） -->
        <div class="graph-detail-panel__section">
          <button type="button" class="graph-detail-panel__section-head" data-section-head="nodes" @click="toggleSection('nodes')">
            <q-icon :name="sectionOpen.nodes ? 'expand_more' : 'chevron_right'" size="16px" />
            <span>{{ t('graphs.detailSectionNodes') }}</span>
            <span class="graph-detail-panel__section-count">{{ graph.nodes?.length ?? 0 }}</span>
          </button>
          <div v-if="sectionOpen.nodes" class="graph-detail-panel__section-body">
            <q-input
              v-model="nodeSearch"
              dense
              outlined
              class="graph-detail-panel__node-search"
              :placeholder="t('graphs.detailNodeSearchPlaceholder')"
            />
            <div v-if="availableTypes.length > 1" class="graph-detail-panel__type-chips">
              <button
                v-for="type in availableTypes"
                :key="type"
                type="button"
                :data-type="type"
                :class="['graph-detail-panel__type-chip', { 'is-active': selectedTypes.has(type) }]"
                :style="chipStyle(type)"
                @click="toggleType(type)"
              >
                {{ nodeTypeLabel(type) }} {{ nodeCounts[type] }}
              </button>
            </div>
            <div
              v-if="filteredNodes.length > 0"
              ref="containerRef"
              class="graph-detail-panel__vlist"
              @scroll="onScroll"
            >
              <div class="graph-detail-panel__vlist-spacer" :style="{ height: `${totalHeight}px` }">
                <div
                  v-for="vr in visibleRows"
                  :key="vr.item.id"
                  class="graph-detail-panel__node-row"
                  :style="{ top: `${vr.top}px` }"
                >
                  <span class="graph-detail-panel__node-dot" :style="{ background: nodeTypeBorderColor(vr.item.type) }" />
                  <span class="graph-detail-panel__node-id" :title="vr.item.id">{{ vr.item.id }}</span>
                  <span class="graph-detail-panel__node-type">{{ nodeTypeLabel(vr.item.type) }}</span>
                  <button
                    type="button"
                    class="graph-detail-panel__node-locate"
                    data-action="locate"
                    @click.stop="emit('locateNode', vr.item.id)"
                  >
                    {{ t('graphs.detailNodeLocate') }}
                  </button>
                </div>
              </div>
            </div>
            <div v-else class="graph-detail-panel__section-empty">{{ t('graphs.detailNodesEmpty') }}</div>
          </div>
        </div>

        <!-- 状态字段 section（可折叠，前 20 + 管理全部） -->
        <div v-if="(graph.stateFields?.length ?? 0) > 0" class="graph-detail-panel__section">
          <button type="button" class="graph-detail-panel__section-head" data-section-head="fields" @click="toggleSection('fields')">
            <q-icon :name="sectionOpen.fields ? 'expand_more' : 'chevron_right'" size="16px" />
            <span>{{ t('graphs.detailSectionStateFields') }}</span>
            <span class="graph-detail-panel__section-count">{{ graph.stateFields.length }}</span>
          </button>
          <div v-if="sectionOpen.fields" class="graph-detail-panel__section-body">
            <div v-for="field in visibleStateFields" :key="field.name" class="graph-detail-panel__field-row">
              <span class="graph-detail-panel__state-name">{{ field.name }}</span>
              <span class="graph-detail-panel__state-type">{{ fieldTypeLabel(field.type) }}</span>
              <span class="graph-detail-panel__state-reducer">{{ reducerLabel(field.reducer) }}</span>
            </div>
            <button
              v-if="graph.stateFields.length > STATE_FIELD_PREVIEW_COUNT"
              type="button"
              class="graph-detail-panel__more-link"
              data-action="manage-schema"
              @click="emit('manageSchema')"
            >
              {{ t('graphs.detailManageAllFields', { count: graph.stateFields.length }) }}
            </button>
          </div>
        </div>

        <!-- R2-B.6 执行历史 section（可折叠，最近 5 条 + 全部） -->
        <div class="graph-detail-panel__section">
          <button type="button" class="graph-detail-panel__section-head" data-section-head="runs" @click="toggleSection('runs')">
            <q-icon :name="sectionOpen.runs ? 'expand_more' : 'chevron_right'" size="16px" />
            <span>{{ t('graphs.detailSectionRuns') }}</span>
            <span class="graph-detail-panel__section-count">{{ execCountLabel }}</span>
          </button>
          <div v-if="sectionOpen.runs" class="graph-detail-panel__section-body">
            <template v-if="recentExecutions.length > 0">
              <div v-for="exec in recentExecutions" :key="exec.executionId" class="graph-detail-panel__exec-row">
                <q-icon :name="stepIcon(exec.status)" :color="stepColor(exec.status)" size="14px" />
                <span class="graph-detail-panel__exec-status">{{ execStatusLabel(exec.status) }}</span>
                <span class="graph-detail-panel__exec-time">{{ relativeTime(exec.startedAt) }}</span>
              </div>
              <button type="button" class="graph-detail-panel__more-link" data-action="view-executions" @click="emit('viewExecutions')">
                {{ t('graphs.detailViewAllRuns') }}
              </button>
            </template>
            <div v-else class="graph-detail-panel__section-empty">{{ t('graphs.detailRunsEmpty') }}</div>
          </div>
        </div>
      </div>
    </template>

    <div v-else class="graph-detail-panel__empty">
      <q-icon name="touch_app" size="32px" color="grey-6" />
      <div class="graph-detail-panel__empty-text">{{ t('graphs.detailEmptyHint') }}</div>
      <div class="graph-detail-panel__empty-hint">{{ t('graphs.detailEmptySubHint') }}</div>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { GraphDefinition, GraphExecutionSummary, NodeDef, NodeType } from '../../features/graph/types';
import { EXECUTION_STATUS_STYLES, NODE_TYPE_STYLES, REDUCER_OPTIONS, STATE_FIELD_TYPE_OPTIONS, normalizeStateFieldType } from '../../features/graph/types';
import { deriveGraphStatus, GRAPH_CARD_STATUS_LABEL_KEYS, relativeTime, stepColor, stepIcon } from '../../features/graph/utils';
import { useVirtualRows } from '../../features/graph/useVirtualRows';

const NODE_ROW_HEIGHT = 32;
const STATE_FIELD_PREVIEW_COUNT = 20;
const EXEC_PREVIEW_COUNT = 5;
const SECTION_STORAGE_KEYS = {
  nodes: 'graph.detail.section.nodes',
  fields: 'graph.detail.section.fields',
  runs: 'graph.detail.section.runs',
} as const;

type SectionKey = keyof typeof SECTION_STORAGE_KEYS;

const props = defineProps<{
  graph: GraphDefinition | null;
  isDark: boolean;
  nodeCounts: Partial<Record<NodeType, number>>;
  nodeTypeBorderColor: (type: string) => string;
  executions?: GraphExecutionSummary[];
  executionsHasMore?: boolean;
}>();

const emit = defineEmits<{
  close: [];
  edit: [id: string];
  run: [graph: GraphDefinition];
  duplicate: [graph: GraphDefinition];
  export: [graph: GraphDefinition];
  delete: [graph: GraphDefinition];
  locateNode: [nodeId: string];
  manageSchema: [];
  viewExecutions: [];
}>();

const { t } = useI18n();

// ---- 头部状态徽章（R2-A 同源派生） ----
const status = computed(() => deriveGraphStatus(props.graph ?? { nodes: [] }));

// ---- 统计行 ----
const executions = computed(() => props.executions ?? []);
const execCountLabel = computed(() => {
  const n = executions.value.length;
  return props.executionsHasMore ? `${n}+` : `${n}`;
});
const recentExecutions = computed(() => executions.value.slice(0, EXEC_PREVIEW_COUNT));

// ---- 节点 section：搜索 + 类型过滤 + 虚拟滚动 ----
const nodeSearch = ref('');
const selectedTypes = ref<Set<string>>(new Set());

const availableTypes = computed(() =>
  (Object.keys(props.nodeCounts) as NodeType[]).filter((type) => (props.nodeCounts[type] ?? 0) > 0),
);

const filteredNodes = computed<NodeDef[]>(() => {
  const nodes = props.graph?.nodes ?? [];
  const q = nodeSearch.value.trim().toLowerCase();
  const types = selectedTypes.value;
  return nodes.filter((n) => {
    if (types.size > 0 && !types.has(n.type)) return false;
    if (!q) return true;
    return n.id.toLowerCase().includes(q) || (n.description ?? '').toLowerCase().includes(q);
  });
});

function toggleType(type: string) {
  const next = new Set(selectedTypes.value);
  if (next.has(type)) {
    next.delete(type);
  } else {
    next.add(type);
  }
  selectedTypes.value = next;
}

function chipStyle(type: string) {
  const color = props.nodeTypeBorderColor(type);
  return { borderColor: color, color };
}

const { containerRef, visibleRows, totalHeight, onScroll } = useVirtualRows({
  rows: filteredNodes,
  rowHeight: NODE_ROW_HEIGHT,
  buffer: 5,
});

// ---- 状态字段 section：前 20 条预览 ----
const visibleStateFields = computed(() => (props.graph?.stateFields ?? []).slice(0, STATE_FIELD_PREVIEW_COUNT));

// ---- 折叠态持久化（localStorage）：默认全部折叠，展开后记住 ----
function readSectionOpen(key: SectionKey): boolean {
  try {
    return localStorage.getItem(SECTION_STORAGE_KEYS[key]) === 'open';
  } catch {
    return false;
  }
}

const sectionOpen = reactive<Record<SectionKey, boolean>>({
  nodes: readSectionOpen('nodes'),
  fields: readSectionOpen('fields'),
  runs: readSectionOpen('runs'),
});

function toggleSection(key: SectionKey) {
  sectionOpen[key] = !sectionOpen[key];
  try {
    localStorage.setItem(SECTION_STORAGE_KEYS[key], sectionOpen[key] ? 'open' : 'collapsed');
  } catch {
    // localStorage 不可用时静默降级为会话内状态
  }
}

function nodeTypeLabel(type: string): string {
  const cfg = (NODE_TYPE_STYLES as Record<string, { labelKey?: string }>)[type];
  return cfg?.labelKey ? t(cfg.labelKey) : type;
}

// ---- 枚举值 i18n 映射（禁止裸渲染英文枚举） ----
function execStatusLabel(status: string): string {
  const labelKey = EXECUTION_STATUS_STYLES[status]?.labelKey;
  return labelKey ? t(labelKey) : status;
}

function fieldTypeLabel(type: string): string {
  const normalized = normalizeStateFieldType(type);
  const labelKey = STATE_FIELD_TYPE_OPTIONS.find((o) => o.value === normalized)?.labelKey;
  return labelKey ? t(labelKey) : type;
}

function reducerLabel(reducer: string): string {
  const labelKey = REDUCER_OPTIONS.find((o) => o.value === reducer)?.labelKey;
  return labelKey ? t(labelKey) : reducer;
}
</script>
