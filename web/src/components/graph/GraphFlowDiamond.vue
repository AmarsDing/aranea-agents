<template>
  <div
    :class="[
      'graph-flow-diamond',
      `graph-flow-diamond--${data.nodeType}`,
      {
        'graph-flow-diamond--selected': selected,
        'graph-flow-diamond--running': data.execStatus === 'running',
        'graph-flow-diamond--completed': data.execStatus === 'completed',
        'graph-flow-diamond--failed': data.execStatus === 'error' || data.execStatus === 'failed',
        'graph-flow-diamond--interrupted': data.execStatus === 'interrupted',
      },
    ]"
    role="group"
    :aria-label="data.label || data.nodeId"
    :style="diamondStyle"
  >
    <Handle type="target" :position="Position.Top" :style="{ left: '50%', top: '0' }" />
    <div class="graph-flow-diamond__inner">
      <q-icon :name="styleConfig.icon" size="14px" />
      <span class="graph-flow-diamond__label">{{ data.label || data.nodeId }}</span>
      <q-badge
        v-if="data.execStatus && data.execStatus !== 'idle'"
        dense
        rounded
        class="graph-flow-diamond__status-badge"
        :class="diamondStatusBadgeClass"
      >
        {{
          data.execStatus === 'running'
            ? '运行'
            : data.execStatus === 'completed'
              ? '完成'
              : data.execStatus === 'failed' || data.execStatus === 'error'
                ? '失败'
                : data.execStatus === 'interrupted'
                  ? '中断'
                  : data.execStatus
        }}
      </q-badge>
    </div>
    <Handle type="source" :position="Position.Bottom" :style="{ left: '50%', bottom: '0' }" />
    <Handle
      v-if="data.nodeType === 'router'"
      id="branch-yes"
      type="source"
      :position="Position.Right"
      :style="{ right: '0', top: '50%' }"
    />
    <Handle
      v-if="data.nodeType === 'router'"
      id="branch-no"
      type="source"
      :position="Position.Left"
      :style="{ left: '0', top: '50%' }"
    />
    <span
      v-if="data.nodeType === 'router'"
      class="graph-flow-diamond__branch-label graph-flow-diamond__branch-label--yes"
      >是</span
    >
    <span
      v-if="data.nodeType === 'router'"
      class="graph-flow-diamond__branch-label graph-flow-diamond__branch-label--no"
      >否</span
    >
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { Handle, Position } from '@vue-flow/core';
import { NODE_TYPE_STYLES, type NodeType } from '../../features/graph/types';

const props = defineProps<{
  id: string;
  data: {
    nodeId: string;
    nodeType: NodeType;
    label: string;
    execStatus?: string;
  };
  selected?: boolean;
}>();

const styleConfig = computed(() => NODE_TYPE_STYLES[props.data.nodeType] ?? NODE_TYPE_STYLES.router);

const diamondStyle = computed(() => ({
  '--node-fill': styleConfig.value.fillColor,
  '--node-border': styleConfig.value.borderColor,
}));

const diamondStatusBadgeClass = computed(() => {
  const s = props.data.execStatus;
  if (s === 'running') return 'graph-flow-diamond__status-badge--running';
  if (s === 'completed') return 'graph-flow-diamond__status-badge--completed';
  if (s === 'error' || s === 'failed') return 'graph-flow-diamond__status-badge--failed';
  if (s === 'interrupted') return 'graph-flow-diamond__status-badge--interrupted';
  return '';
});
</script>
