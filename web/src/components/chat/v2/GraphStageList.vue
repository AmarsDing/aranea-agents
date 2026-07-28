<!-- web/src/components/chat/v2/GraphStageList.vue
  GraphStageBlock 的移动降级视图（设计 72 §3.2：DAG 画布 → 分阶段折叠列表）：
  - 数据全复用（activityV2Store / useGraphNodeTeam / graphTeamNodeUi），仅视图新写
  - 节点按 DAG 拓扑序（层 → 层内序）纵向排列；每节点卡片含成员/状态/进度
  - 运行/失败/中断节点默认展开，其余收起；成员行点击 → MemberSessionDialog
-->
<template>
  <div class="graph-stage-list" :data-graph-stage-id="graphStage.ID">
    <div class="graph-stage-header">
      <span class="header-label">
        <q-icon name="account_tree" size="16px" class="header-icon" />
        {{ t('chat.v2.graphStageTitle') }}
      </span>
      <span class="header-progress">{{ completedCount }}/{{ nodes.length }}</span>
      <q-badge :color="stageStatusColor">{{ stageStatusLabel }}</q-badge>
    </div>

    <div class="gsl-nodes">
      <div v-for="node in orderedNodes" :key="node.ID" class="gsl-node" :class="`gsl-node--${node.Status}`">
        <div class="gsl-node__header" @click="toggleNode(node.ID, node.Status)">
          <span class="gsl-node__dot" :class="`gsl-node__dot--${toneOf(node.Status)}`" />
          <span class="gsl-node__label" :title="node.Label">{{ node.Label }}</span>
          <span v-if="membersOf(node).length > 0" class="gsl-node__progress">
            {{ progressOf(node).completed }}/{{ membersOf(node).length }}
          </span>
          <span class="gsl-node__status" :class="`gsl-node__status--${toneOf(node.Status)}`">
            {{ nodeStatusLabel(node.Status) }}
          </span>
          <q-icon
            name="expand_more"
            size="18px"
            class="gsl-node__chevron"
            :class="{ 'gsl-node__chevron--open': isExpanded(node) }"
          />
        </div>

        <q-slide-transition>
          <div v-show="isExpanded(node)" class="gsl-node__body">
            <template v-if="membersOf(node).length > 0">
              <div
                v-for="ms in membersOf(node)"
                :key="ms.ID"
                class="gsl-member"
                :class="{ 'gsl-member--active': ms.Status === 'running' }"
                :data-member-id="ms.ID"
                @click.stop="onSelectMember(ms)"
              >
                <span class="gsl-member__dot" :class="`gsl-member__dot--${toneOf(ms.Status)}`" />
                <span class="gsl-member__name" :title="memberName(ms)">{{ memberName(ms) }}</span>
                <span class="gsl-member__meta" :class="`gsl-member__meta--${toneOf(ms.Status)}`">
                  {{ memberMeta(ms) }}
                </span>
              </div>
            </template>
            <div v-else class="gsl-member gsl-member--empty">
              <span class="gsl-member__dot gsl-member__dot--muted" />
              <span class="gsl-member__name">{{ t('chat.v2.noMembers') }}</span>
            </div>

            <div v-if="statusRowOf(node)" class="gsl-node__status-row">
              <div v-if="statusRowOf(node)!.kind === 'error'" class="gsl-error" :title="statusRowOf(node)!.title">
                <q-icon name="error" size="12px" />
                <span class="gsl-node__status-row-text">{{ statusRowOf(node)!.text }}</span>
              </div>
              <div v-else class="gsl-action" :title="statusRowOf(node)!.text">
                <q-icon :name="statusRowOf(node)!.icon" size="12px" />
                <span class="gsl-node__status-row-text">{{ statusRowOf(node)!.text }}</span>
              </div>
            </div>
          </div>
        </q-slide-transition>
      </div>
    </div>

    <MemberSessionDialog
      v-model:open="memberDialogOpen"
      :member-session="activeMember"
      @pause-agent="(sid) => $emit('pause-agent', sid)"
      @inject-agent="(p) => $emit('inject-agent', p)"
      @expand="(ids) => $emit('expand', ids)"
      @confirm-step="(p) => $emit('confirm-step', p)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useActivityQueries } from '../../../features/chat/composables/useActivityQueries';
import { useGraphNodeTeam } from '../../../features/chat/composables/useGraphNodeTeam';
import { usePlanDAGLayout } from '../../../features/chat/composables/usePlanDAGLayout';
import type { GraphNode, GraphStage, MemberSession } from '../../../features/chat/v2Types';
import type { ConfirmStepPayload } from '../../../features/chat/types';
import MemberSessionDialog from './MemberSessionDialog.vue';
import { formatMemberDuration, graphTeamNodeStatusText, graphTeamNodeTone } from './graphTeamNodeUi';
import { defaultGraphNodeExpanded, deriveGraphStageStatus, orderGraphNodesForList } from './graphStageListUi';

function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string) => key };
  }
}

const props = defineProps<{ graphStage: GraphStage }>();
defineEmits<{
  'pause-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
  expand: [sessionIds: string[]];
  'confirm-step': [payload: ConfirmStepPayload];
}>();

const { t } = useSafeI18n();
const store = useActivityQueries();
const { membersOf } = useGraphNodeTeam();

// ── 节点：store 实时查询 + DAG 拓扑序（层 → 层内序） ──
const nodes = computed(() => store.getGraphStageNodes(props.graphStage.ID));

const { layoutDAG } = usePlanDAGLayout();
const layoutResult = computed(() => layoutDAG(nodes.value, { width: 0 }));
const orderedNodes = computed(() =>
  orderGraphNodesForList(nodes.value, layoutResult.value.layers, layoutResult.value.orderInLayer),
);

const completedCount = computed(() => nodes.value.filter((n) => n.Status === 'completed').length);

// ── 容器状态（与 GraphStageBlock 同规则，经共享纯函数） ──
const derivedStatus = computed(() => deriveGraphStageStatus(props.graphStage.Status, nodes.value));

const stageStatusColor = computed(
  () =>
    ({
      running: 'blue',
      completed: 'green',
      failed: 'red',
      interrupted: 'yellow-8',
    })[derivedStatus.value] || 'grey',
);

const stageStatusLabel = computed(() => {
  const map: Record<string, string> = {
    running: t('chat.v2.statusRunning'),
    completed: t('chat.v2.statusCompleted'),
    failed: t('chat.v2.statusFailed'),
    interrupted: t('chat.v2.statusInterrupted'),
  };
  return map[derivedStatus.value] || derivedStatus.value;
});

// ── 折叠态：用户显式展开/收起覆盖默认值（running/failed/interrupted 默认展开） ──
const expandedOverrides = ref(new Map<string, boolean>());

function isExpanded(node: GraphNode): boolean {
  return expandedOverrides.value.get(node.ID) ?? defaultGraphNodeExpanded(node.Status);
}

function toggleNode(nodeId: string, status: string) {
  const next = new Map(expandedOverrides.value);
  next.set(nodeId, !(expandedOverrides.value.get(nodeId) ?? defaultGraphNodeExpanded(status)));
  expandedOverrides.value = next;
}

// ── 节点展示派生（复用 graphTeamNodeUi 纯函数） ──
const toneOf = graphTeamNodeTone;

function nodeStatusLabel(status: string): string {
  const map: Record<string, string> = {
    pending: t('chat.v2.statusPending'),
    running: t('chat.v2.statusRunning'),
    completed: t('chat.v2.statusCompleted'),
    failed: t('chat.v2.statusFailed'),
    interrupted: t('chat.v2.statusInterrupted'),
  };
  return map[status] || status;
}

function progressOf(node: { ID: string }) {
  const members = membersOf(node);
  return { completed: members.filter((ms) => ms.Status === 'completed').length, total: members.length };
}

function statusRowOf(node: { ID: string }) {
  return graphTeamNodeStatusText(
    membersOf(node),
    (ms) => {
      const steps = store.getMemberSessionSteps(ms);
      const last = steps.length > 0 ? steps[steps.length - 1] : null;
      return last ? { Kind: last.Kind, Status: last.Status, ToolName: last.ToolName } : null;
    },
    {
      running: t('chat.v2.statusRunning'),
      paused: t('chat.v2.statusPaused'),
      failed: t('chat.v2.statusFailed'),
    },
  );
}

function memberName(ms: MemberSession): string {
  return ms.AgentName || ms.AgentKey;
}

/** 成员右侧元信息：完成 → 总耗时；其余 → 状态文案（列表摘要不逐秒跳表）。 */
function memberMeta(ms: MemberSession): string {
  if (ms.Status === 'completed') {
    const startMs = Date.parse(ms.StartedAt);
    const endMs = ms.FinishedAt ? Date.parse(ms.FinishedAt) : NaN;
    if (!Number.isNaN(startMs) && !Number.isNaN(endMs)) return formatMemberDuration(endMs - startMs);
    return t('chat.v2.statusCompleted');
  }
  const map: Record<string, string> = {
    running: t('chat.v2.statusRunning'),
    paused: t('chat.v2.statusPaused'),
    failed: t('chat.v2.statusFailed'),
    skipped: t('chat.v2.statusSkipped'),
  };
  return map[ms.Status] || t('chat.v2.statusPending');
}

// ── 成员弹框（实时查询，模式同 GraphStageBlock） ──
const memberDialogOpen = ref(false);
const activeMemberId = ref<string | null>(null);

const activeMember = computed<MemberSession | null>(() => {
  const id = activeMemberId.value;
  return id ? (store.memberSessions().get(id) ?? null) : null;
});

function onSelectMember(ms: MemberSession) {
  activeMemberId.value = ms.ID;
  memberDialogOpen.value = true;
}
</script>

<style lang="sass" scoped>
.graph-stage-list
  padding: 8px 0
  margin: 8px 0

.graph-stage-header
  display: flex
  align-items: center
  gap: 8px
  margin-bottom: 8px
  font-size: 13px
  font-weight: 600
  color: var(--color-text-primary)

.header-icon
  margin-right: 4px
  color: var(--q-primary)

.header-label
  flex: 1

.header-progress
  font-size: 12px
  font-weight: 500
  color: var(--color-text-secondary)

// ── 节点卡片（触控友好：整行可点，最小高度 44px） ──
.gsl-nodes
  display: flex
  flex-direction: column
  gap: 8px

.gsl-node
  border: 1px solid var(--glass-border)
  border-left: 3px solid var(--node-accent, var(--glass-border))
  border-radius: 10px
  background: var(--glass-surface)
  overflow: hidden

.gsl-node--running
  --node-accent: var(--q-primary)

.gsl-node--completed
  --node-accent: var(--color-success)

.gsl-node--failed
  --node-accent: var(--color-danger)

.gsl-node--interrupted
  --node-accent: var(--color-warning)

.gsl-node__header
  display: flex
  align-items: center
  gap: 8px
  min-height: 44px
  padding: 8px 12px
  cursor: pointer
  user-select: none

.gsl-node__dot
  flex: 0 0 auto
  width: 8px
  height: 8px
  border-radius: 50%

.gsl-node__label
  flex: 1 1 auto
  min-width: 0
  font-size: 13px
  font-weight: 600
  color: var(--color-text-primary)
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis

.gsl-node__progress
  flex: 0 0 auto
  font-size: 11px
  color: var(--color-text-tertiary)
  font-variant-numeric: tabular-nums

.gsl-node__status
  flex: 0 0 auto
  padding: 2px 8px
  border-radius: 999px
  font-size: 10px
  font-weight: 600

.gsl-node__chevron
  flex: 0 0 auto
  color: var(--color-text-tertiary)
  transition: transform 0.2s ease

  &--open
    transform: rotate(180deg)

.gsl-node__body
  padding: 4px 12px 10px
  border-top: 1px solid var(--glass-border)

// ── 成员行 ──
.gsl-member
  display: flex
  align-items: center
  gap: 8px
  min-height: 40px
  padding: 4px 6px
  border-radius: 8px
  cursor: pointer
  user-select: none

  &:active
    background: var(--glass-surface-hover)

  &--active
    background: color-mix(in srgb, var(--q-primary) 7%, transparent)

  &--empty
    cursor: default
    color: var(--color-text-tertiary)

.gsl-member__dot
  flex: 0 0 auto
  width: 8px
  height: 8px
  border-radius: 50%

.gsl-member__name
  flex: 1 1 auto
  min-width: 0
  font-size: 12px
  font-weight: 500
  color: var(--color-text-primary)
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis

.gsl-member--empty .gsl-member__name
  color: var(--color-text-tertiary)

.gsl-member__meta
  flex: 0 0 auto
  font-size: 11px
  color: var(--color-text-tertiary)
  font-variant-numeric: tabular-nums

// ── 色调（dot / 状态徽章 / meta 共用） ──
.gsl-node__dot--accent,
.gsl-member__dot--accent
  background: var(--q-primary)

.gsl-node__dot--success,
.gsl-member__dot--success
  background: var(--color-success)

.gsl-node__dot--danger,
.gsl-member__dot--danger
  background: var(--color-danger)

.gsl-node__dot--warning,
.gsl-member__dot--warning
  background: var(--color-warning)

.gsl-node__dot--muted,
.gsl-member__dot--muted
  background: var(--color-text-tertiary)

.gsl-node__status--accent
  color: var(--q-primary)
  background: color-mix(in srgb, var(--q-primary) 13%, transparent)

.gsl-node__status--success
  color: var(--color-success)
  background: color-mix(in srgb, var(--color-success) 14%, transparent)

.gsl-node__status--danger
  color: var(--color-danger)
  background: color-mix(in srgb, var(--color-danger) 13%, transparent)

.gsl-node__status--warning
  color: var(--color-warning)
  background: color-mix(in srgb, var(--color-warning) 15%, transparent)

.gsl-node__status--muted
  color: var(--color-text-tertiary)
  background: color-mix(in srgb, var(--color-text-tertiary) 12%, transparent)

.gsl-member__meta--danger
  color: var(--color-danger)

.gsl-member__meta--warning
  color: var(--color-warning)

// ── 状态行（错误摘要 / 当前动作） ──
.gsl-node__status-row
  display: flex
  align-items: center
  margin-top: 4px
  font-size: 11px

.gsl-action,
.gsl-error
  display: flex
  align-items: center
  gap: 4px
  min-width: 0
  max-width: 100%
  padding: 3px 8px
  border-radius: 6px

.gsl-action
  color: var(--q-primary)
  background: color-mix(in srgb, var(--q-primary) 10%, transparent)

.gsl-error
  color: var(--color-danger)
  background: color-mix(in srgb, var(--color-danger) 10%, transparent)

.gsl-node__status-row-text
  min-width: 0
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis

@media (prefers-reduced-motion: reduce)
  .gsl-node__chevron
    transition: none
</style>
