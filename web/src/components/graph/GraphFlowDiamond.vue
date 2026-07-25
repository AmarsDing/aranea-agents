<template>
  <div class="graph-flow-diamond-wrap" role="group" :aria-label="data.label || data.nodeId">
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
          'graph-flow-diamond--issue-error': data.issue?.level === 'error',
          'graph-flow-diamond--issue-warning': data.issue?.level === 'warning',
        },
      ]"
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
          {{ statusLabel }}
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
        >{{ t('graphs.flowBranchYes') }}</span
      >
      <span
        v-if="data.nodeType === 'router'"
        class="graph-flow-diamond__branch-label graph-flow-diamond__branch-label--no"
        >{{ t('graphs.flowBranchNo') }}</span
      >
    </div>
    <div v-if="data.spotlighted && data.issue" class="graph-flow-diamond__bubble">
      <div class="graph-flow-diamond__bubble-code">{{ data.issue.code }}</div>
      <div class="graph-flow-diamond__bubble-message">{{ data.issue.message }}</div>
      <div v-if="issueSuggestion" class="graph-flow-diamond__bubble-suggestion">
        <q-icon name="lightbulb" size="12px" />
        <span>{{ issueSuggestion }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { Handle, Position } from '@vue-flow/core';
import { NODE_TYPE_STYLES, EXECUTION_STATUS_STYLES, type NodeType, type NodeIssueInfo } from '../../features/graph/types';
import { validationSuggestionKey } from '../../features/graph/validationIssues';

const { t } = useI18n();

const props = defineProps<{
  id: string;
  data: {
    nodeId: string;
    nodeType: NodeType;
    label: string;
    execStatus?: string;
    issue?: NodeIssueInfo;
    spotlighted?: boolean;
  };
  selected?: boolean;
}>();

const styleConfig = computed(() => NODE_TYPE_STYLES[props.data.nodeType] ?? NODE_TYPE_STYLES.router);

const diamondStyle = computed(() => ({
  '--node-fill': styleConfig.value.fillColor,
  '--node-border': styleConfig.value.borderColor,
}));

const statusLabel = computed(() => {
  const status = props.data.execStatus ?? 'idle';
  const cfg = EXECUTION_STATUS_STYLES[status] ?? EXECUTION_STATUS_STYLES.idle;
  return t(cfg.labelKey);
});

const diamondStatusBadgeClass = computed(() => {
  const s = props.data.execStatus;
  if (s === 'running') return 'graph-flow-diamond__status-badge--running';
  if (s === 'completed') return 'graph-flow-diamond__status-badge--completed';
  if (s === 'error' || s === 'failed') return 'graph-flow-diamond__status-badge--failed';
  if (s === 'interrupted') return 'graph-flow-diamond__status-badge--interrupted';
  return '';
});

const issueSuggestion = computed(() => {
  const issue = props.data.issue;
  if (!issue) return '';
  const key = validationSuggestionKey(issue.code);
  return key ? t(key) : '';
});
</script>
