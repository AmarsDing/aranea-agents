<template>
  <aside :class="['graph-detail-panel', { 'is-dark': isDark }]">
    <template v-if="graph">
      <div class="graph-detail-panel__header">
        <div class="graph-detail-panel__title">{{ graph.name }}</div>
        <q-btn flat dense round icon="close" @click="emit('close')" />
      </div>

      <div class="graph-detail-panel__body">
        <div v-if="graph.description" class="graph-detail-panel__desc">{{ graph.description }}</div>

        <div class="graph-detail-panel__section">
          <div class="graph-detail-panel__section-title">{{ t('graphs.detailSectionBasic') }}</div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">{{ t('graphs.detailLabelEngine') }}</span>
            <span class="graph-detail-panel__value">{{
              graph.executionEngine === 'dag' ? t('graphs.engineDAG') : t('graphs.engineBSP')
            }}</span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">{{ t('graphs.detailLabelVersion') }}</span>
            <span class="graph-detail-panel__value">v{{ graph.version }}</span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">{{ t('graphs.detailLabelEntry') }}</span>
            <span class="graph-detail-panel__value graph-detail-panel__value--mono">{{ graph.entryPoint || '—' }}</span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">{{ t('graphs.detailLabelFinish') }}</span>
            <span class="graph-detail-panel__value graph-detail-panel__value--mono">{{
              graph.finishPoint || '—'
            }}</span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">{{ t('graphs.detailLabelCheckpoint') }}</span>
            <span class="graph-detail-panel__value">{{
              graph.enableCheckpoint ? t('graphs.detailCheckpointEnabled') : t('graphs.detailCheckpointDisabled')
            }}</span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">{{ t('graphs.detailLabelCreatedAt') }}</span>
            <span class="graph-detail-panel__value">{{ formatDate(graph.createdAt) }}</span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">{{ t('graphs.detailLabelUpdatedAt') }}</span>
            <span class="graph-detail-panel__value">{{ formatDate(graph.updatedAt) }}</span>
          </div>
        </div>

        <div class="graph-detail-panel__section">
          <div class="graph-detail-panel__section-title">{{ t('graphs.detailSectionStats') }}</div>
          <div class="graph-detail-panel__stats">
            <div
              v-for="(count, type) in nodeCounts"
              :key="type"
              class="graph-detail-panel__stat-chip"
              :style="{ borderColor: nodeTypeBorderColor(type as string), color: nodeTypeBorderColor(type as string) }"
            >
              {{ (NODE_TYPE_EMOJI as any)[type] }} {{ nodeTypeLabel(type as string) }} ×{{ count }}
            </div>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">{{ t('graphs.detailLabelNodes') }}</span>
            <span class="graph-detail-panel__value">{{ graph.nodes?.length ?? 0 }}</span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">{{ t('graphs.detailLabelEdges') }}</span>
            <span class="graph-detail-panel__value">{{ graph.edges?.length ?? 0 }}</span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">{{ t('graphs.detailLabelConditionalEdges') }}</span>
            <span class="graph-detail-panel__value">{{ graph.conditionalEdges?.length ?? 0 }}</span>
          </div>
        </div>

        <div v-if="graph.stateFields?.length" class="graph-detail-panel__section">
          <div class="graph-detail-panel__section-title">{{ t('graphs.detailSectionStateFields') }}</div>
          <div v-for="field in graph.stateFields" :key="field.name" class="graph-detail-panel__state-row">
            <span class="graph-detail-panel__state-name">{{ field.name }}</span>
            <span class="graph-detail-panel__state-type">{{ field.type }}</span>
            <span class="graph-detail-panel__state-reducer">{{ field.reducer }}</span>
          </div>
        </div>

        <div class="graph-detail-panel__actions">
          <q-btn
            unelevated
            rounded
            icon="edit"
            :label="t('graphs.detailActionEdit')"
            class="graph-detail-panel__action-btn"
            @click="emit('edit', graph.id)"
          />
          <q-btn flat rounded icon="play_arrow" :label="t('graphs.detailActionRun')" @click="emit('run', graph)" />
          <q-btn
            flat
            rounded
            icon="content_copy"
            :label="t('graphs.detailActionDuplicate')"
            @click="emit('duplicate', graph)"
          />
          <q-btn
            flat
            rounded
            icon="delete"
            :label="t('graphs.detailActionDelete')"
            color="negative"
            @click="emit('delete', graph)"
          />
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
import { useI18n } from 'vue-i18n';
import type { GraphDefinition, NodeType } from '../../features/graph/types';
import { NODE_TYPE_STYLES } from '../../features/graph/types';

defineProps<{
  graph: GraphDefinition | null;
  isDark: boolean;
  nodeCounts: Partial<Record<NodeType, number>>;
  nodeTypeBorderColor: (type: string) => string;
}>();

const emit = defineEmits<{
  close: [];
  edit: [id: string];
  run: [graph: GraphDefinition];
  duplicate: [graph: GraphDefinition];
  delete: [graph: GraphDefinition];
}>();

const { t } = useI18n();

const NODE_TYPE_EMOJI: Record<NodeType, string> = {
  agent: '🤖',
  llm: '🧠',
  router: '🔀',
  function: '⚙️',
  tool: '🔧',
  join: '🔗',
  hitl: '✋',
};

function nodeTypeLabel(type: string): string {
  const cfg = (NODE_TYPE_STYLES as Record<string, { labelKey?: string }>)[type];
  return cfg?.labelKey ? t(cfg.labelKey) : type;
}

function formatDate(iso: string) {
  if (!iso) return '—';
  const d = new Date(iso);
  return d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
}

void emit;
</script>
