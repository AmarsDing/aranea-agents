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
          <div class="graph-detail-panel__section-title">基本信息</div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">引擎</span>
            <span class="graph-detail-panel__value">{{
              graph.executionEngine === 'dag' ? 'DAG（并行）' : 'BSP（默认）'
            }}</span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">版本</span>
            <span class="graph-detail-panel__value">v{{ graph.version }}</span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">入口</span>
            <span class="graph-detail-panel__value graph-detail-panel__value--mono">{{ graph.entryPoint || '—' }}</span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">结束</span>
            <span class="graph-detail-panel__value graph-detail-panel__value--mono">{{
              graph.finishPoint || '—'
            }}</span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">检查点</span>
            <span class="graph-detail-panel__value">{{ graph.enableCheckpoint ? '已启用' : '未启用' }}</span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">创建时间</span>
            <span class="graph-detail-panel__value">{{ formatDate(graph.createdAt) }}</span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">更新时间</span>
            <span class="graph-detail-panel__value">{{ formatDate(graph.updatedAt) }}</span>
          </div>
        </div>

        <div class="graph-detail-panel__section">
          <div class="graph-detail-panel__section-title">节点统计</div>
          <div class="graph-detail-panel__stats">
            <div
              v-for="(count, type) in nodeCounts"
              :key="type"
              class="graph-detail-panel__stat-chip"
              :style="{ borderColor: nodeTypeBorderColor(type as string), color: nodeTypeBorderColor(type as string) }"
            >
              {{ (NODE_TYPE_EMOJI as any)[type] }} {{ (NODE_TYPE_STYLES as any)[type]?.label ?? type }} ×{{ count }}
            </div>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">节点</span>
            <span class="graph-detail-panel__value">{{ graph.nodes?.length ?? 0 }}</span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">连线</span>
            <span class="graph-detail-panel__value">{{ graph.edges?.length ?? 0 }}</span>
          </div>
          <div class="graph-detail-panel__field">
            <span class="graph-detail-panel__label">条件边</span>
            <span class="graph-detail-panel__value">{{ graph.conditionalEdges?.length ?? 0 }}</span>
          </div>
        </div>

        <div v-if="graph.stateFields?.length" class="graph-detail-panel__section">
          <div class="graph-detail-panel__section-title">状态字段</div>
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
            label="编辑"
            class="graph-detail-panel__action-btn"
            @click="emit('edit', graph.id)"
          />
          <q-btn flat rounded icon="play_arrow" label="执行" @click="emit('run', graph)" />
          <q-btn flat rounded icon="content_copy" label="复制" @click="emit('duplicate', graph)" />
          <q-btn flat rounded icon="delete" label="删除" color="negative" @click="emit('delete', graph)" />
        </div>
      </div>
    </template>

    <div v-else class="graph-detail-panel__empty">
      <q-icon name="touch_app" size="32px" color="grey-6" />
      <div class="graph-detail-panel__empty-text">左键点击卡片查看详情</div>
      <div class="graph-detail-panel__empty-hint">右键点击卡片显示操作菜单</div>
    </div>
  </aside>
</template>

<script setup lang="ts">
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

const NODE_TYPE_EMOJI: Record<NodeType, string> = {
  agent: '🤖',
  llm: '🧠',
  router: '🔀',
  function: '⚙️',
  tool: '🔧',
  join: '🔗',
  hitl: '✋',
};

function formatDate(iso: string) {
  if (!iso) return '—';
  const d = new Date(iso);
  return d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
}
</script>
