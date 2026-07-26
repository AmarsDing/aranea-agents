<template>
  <aside v-if="display" class="team-orchestrate-node-panel">
    <div class="team-orchestrate-node-panel__head">
      <div class="team-orchestrate-node-panel__title">{{ display.displayName }}</div>
      <OrchestrationStatusChip
        :display-status="liveState?.display_status ?? 'waiting'"
        :fine-status="liveState?.status ?? 'idle'"
      />
    </div>
    <div class="team-orchestrate-node-panel__meta">
      <div class="team-orchestrate-node-panel__chip">{{ display.roleLabel }}</div>
    </div>

    <section class="team-orchestrate-node-panel__section">
      <div class="team-orchestrate-node-panel__section-title">收到 · 输入</div>
      <div class="team-orchestrate-node-panel__section-body">{{ inputBody }}</div>
    </section>
    <section class="team-orchestrate-node-panel__section">
      <div class="team-orchestrate-node-panel__section-title">做什么</div>
      <div class="team-orchestrate-node-panel__section-body">{{ doingBody }}</div>
    </section>
    <section class="team-orchestrate-node-panel__section">
      <div class="team-orchestrate-node-panel__section-title">交付 · 输出</div>
      <div class="team-orchestrate-node-panel__section-body">{{ outputBody }}</div>
    </section>

    <div v-if="readOnly && !liveState" class="team-orchestrate-node-panel__foot text-caption text-grey-7">
      正在连接实时状态…
    </div>
  </aside>
  <aside v-else class="team-orchestrate-node-panel team-orchestrate-node-panel--empty">
    <div class="text-subtitle2 q-mb-sm">节点详情</div>
    <div class="text-caption app-text-secondary">点击画布上的 Agent 节点，查看名称、角色与收/做/交说明。</div>
  </aside>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { GraphDefinition } from '../../features/graph/types';
import type { CompileTeamGraphResult } from '../../features/orchestration/compileApi';
import { resolveTeamNodeDisplay } from '../../features/orchestration/teamNodeDisplay';
import type { AgentNodeState } from '../../features/orchestration/types';
import type { TeamDefinition } from '../../features/teams/types';
import OrchestrationStatusChip from '../orchestration/OrchestrationStatusChip.vue';

const props = defineProps<{
  selectedNodeId: string | null;
  graphDef: GraphDefinition;
  compiled: CompileTeamGraphResult | null;
  definition: TeamDefinition | null;
  readOnly: boolean;
  liveState?: AgentNodeState | null;
}>();

const display = computed(() => {
  if (!props.selectedNodeId) return null;
  const node = props.graphDef.nodes.find((n) => n.id === props.selectedNodeId);
  if (!node || node.type !== 'agent') return null;
  return resolveTeamNodeDisplay(node, props.compiled, props.definition);
});

const inputBody = computed(() => props.liveState?.input_preview?.trim() || display.value?.inputHint || '—');

const doingBody = computed(() => {
  const activity = props.liveState?.current_activity;
  if (activity) {
    return activity.display_label?.trim() || activity.tool_name?.trim() || activity.kind?.trim() || '处理中…';
  }
  if (props.liveState?.phase === 'doing') return '处理中…';
  return display.value?.responsibility || '—';
});

const outputBody = computed(
  () =>
    props.liveState?.output_preview?.trim() ||
    props.liveState?.error_message?.trim() ||
    display.value?.outputHint ||
    '—',
);
</script>
