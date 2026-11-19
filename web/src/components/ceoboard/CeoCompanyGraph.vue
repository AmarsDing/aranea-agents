<script setup lang="ts">
// 公司运转图（2026-11-10 方案）：CEO 看板老板视图的只读图形模式。
// 店主 → CEO 汇总 → lane → workflow → node；往下派活、往上数据日报、
// 边框转圈 = 正在干活（近 5 分钟有数据流动，口径见 companyGraph.ts）。
// 图是「看数」层：无任何审批/写操作，拍板仍回决策区（只有人能点头）。
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Overview } from '../../features/ceoboard/types';
import { buildCompanyGraph, type GraphWorkflow } from '../../features/ceoboard/companyGraph';
import { friendlyReason, healthLabel, laneName } from '../../features/ceoboard/friendly';

const props = defineProps<{ ov: Overview; meName?: string }>();
const emit = defineEmits<{
  openNode: [wfId: string, nodeId: string];
  showDecisions: [];
}>();
const { t } = useI18n();

const graph = computed(() => buildCompanyGraph(props.ov));
const cols = computed(() => Math.max(graph.value.lanes.length, 1));
const stageMinWidth = computed(() => Math.max(660, graph.value.lanes.length * 200));
const viewBox = computed(() => `0 0 ${cols.value * 240} 260`);
const cx = computed(() => (cols.value * 240) / 2);

const ownerInitial = computed(() => (props.meName ?? '').trim().charAt(0).toUpperCase());

// 聚焦是讲解焦点（其余列变暗），不是故障状态；lane 被裁剪/热加载移除时复位。
const focusedLane = ref<string | null>(null);
watch(
  () => props.ov.lanes,
  (lanes) => {
    if (focusedLane.value && !lanes.some((l) => l.biz_line === focusedLane.value)) focusedLane.value = null;
  },
);
function toggleFocus(bizLine: string) {
  focusedLane.value = focusedLane.value === bizLine ? null : bizLine;
}

// 展开状态 = 用户手动覆盖 ?? 默认(红/黄展开)；按 wf.id 记录，轮询刷新不冲掉。
const overrides = ref<Record<string, boolean>>({});
function isExpanded(wf: GraphWorkflow): boolean {
  return overrides.value[wf.id] ?? (wf.health === 'red' || wf.health === 'yellow');
}
function toggleWf(wf: GraphWorkflow) {
  overrides.value[wf.id] = !isExpanded(wf);
}

function wfConclusion(wf: GraphWorkflow): string {
  if (!wf.nodes.length) return t('ceoboard.lane.noNodeData');
  return wf.worstReason ? friendlyReason(wf.worstReason) : t('ceoboard.lane.runningOk', { count: wf.nodes.length });
}
</script>

<template>
  <section class="ceo-graph" :aria-label="t('ceoboard.graph.graphAria')">
    <div v-if="!graph.lanes.length" class="ceo-empty ceo-graph-empty">{{ t('ceoboard.graph.empty') }}</div>
    <div v-else class="ceo-graph-scroll">
      <div class="ceo-graph-stage" :style="{ minWidth: `${stageMinWidth}px` }">
        <div class="ceo-graph-top">
          <svg class="ceo-graph-links" :viewBox="viewBox" preserveAspectRatio="none" aria-hidden="true">
            <path
              class="ceo-graph-line ceo-graph-line--down"
              :d="`M${cx - 16} 66 C${cx - 56} 78 ${cx - 56} 90 ${cx - 16} 100`"
              vector-effect="non-scaling-stroke"
            />
            <path
              class="ceo-graph-line ceo-graph-line--up"
              :d="`M${cx + 16} 100 C${cx + 56} 90 ${cx + 56} 78 ${cx + 16} 66`"
              vector-effect="non-scaling-stroke"
            />
            <g v-for="(lane, i) in graph.lanes" :key="lane.bizLine">
              <path
                class="ceo-graph-line ceo-graph-line--down"
                :d="`M${cx - 6} 208 C${cx - 6} 236 ${(i + 0.5) * 240 - 8} 232 ${(i + 0.5) * 240 - 8} 256`"
                vector-effect="non-scaling-stroke"
              />
              <path
                class="ceo-graph-line ceo-graph-line--up"
                :d="`M${(i + 0.5) * 240 + 8} 256 C${(i + 0.5) * 240 + 8} 236 ${cx + 6} 236 ${cx + 6} 208`"
                vector-effect="non-scaling-stroke"
              />
            </g>
          </svg>

          <div class="ceo-graph-owner ceo-graph-panel">
            <span class="ceo-graph-owner-avatar">
              <template v-if="ownerInitial">{{ ownerInitial }}</template>
              <q-icon v-else name="person" />
            </span>
            <span class="ceo-graph-owner-body">
              <b>{{ meName || t('ceoboard.graph.ownerAnonymous') }}</b>
              <small>{{ t('ceoboard.graph.ownerRole') }}</small>
            </span>
          </div>
          <span class="ceo-graph-edge-label ceo-graph-edge-label--down">{{ t('ceoboard.graph.directionDown') }}</span>
          <span class="ceo-graph-edge-label ceo-graph-edge-label--up">{{ t('ceoboard.graph.reportUp') }}</span>

          <button
            type="button"
            class="ceo-graph-ceo ceo-graph-panel"
            :title="t('ceoboard.graph.showDecisions')"
            @click="emit('showDecisions')"
          >
            <q-icon name="radar" class="ceo-graph-ceo-icon" />
            <span class="ceo-graph-ceo-body">
              <b>CEO Agent</b>
              <small>{{ t('ceoboard.graph.ceoRole') }}</small>
              <span class="ceo-graph-ceo-hint">{{ t('ceoboard.graph.ceoHint') }}</span>
            </span>
            <span class="ceo-graph-ceo-stats">
              <span class="ceo-dot" :class="graph.level"></span>
              <span>{{ t('ceoboard.graph.ceoSummary', { lanes: graph.lanes.length, nodes: graph.totalNodes }) }}</span>
              <span :class="{ 'ceo-graph-pending': graph.approvalCount > 0 }">{{
                t('ceoboard.graph.pending', { count: graph.approvalCount })
              }}</span>
              <span :class="{ 'ceo-alert-num': graph.alertCount > 0 }">{{
                t('ceoboard.graph.alerts', { count: graph.alertCount })
              }}</span>
            </span>
          </button>

          <div class="ceo-graph-legend">
            <p><i class="ceo-graph-legend-line ceo-graph-line--down"></i>{{ t('ceoboard.graph.legendDown') }}</p>
            <p><i class="ceo-graph-legend-line ceo-graph-line--up"></i>{{ t('ceoboard.graph.legendUp') }}</p>
            <p :title="t('ceoboard.graph.workingTip')">
              <i class="ceo-graph-legend-ring"></i>{{ t('ceoboard.graph.legendWorking') }}
            </p>
          </div>
        </div>

        <div class="ceo-graph-lanes" :style="{ gridTemplateColumns: `repeat(${cols}, 1fr)` }">
          <div
            v-for="lane in graph.lanes"
            :key="lane.bizLine"
            class="ceo-graph-lane"
            :class="{ 'ceo-graph-dim': focusedLane !== null && focusedLane !== lane.bizLine }"
          >
            <span class="ceo-graph-up-label ceo-mono">{{ t('ceoboard.graph.upLabel', { count: lane.volume1h }) }}</span>
            <button
              type="button"
              class="ceo-graph-lane-card ceo-graph-panel"
              :class="{ 'ceo-graph-working': lane.working }"
              :aria-pressed="focusedLane === lane.bizLine"
              :title="t('ceoboard.graph.focusTip')"
              @click="toggleFocus(lane.bizLine)"
            >
              <span class="ceo-graph-lane-head">
                <span class="ceo-dot" :class="lane.health"></span>
                <b>{{ laneName(lane.bizLine) }}</b>
                <span class="ceo-biz-pill" :class="lane.health">{{ healthLabel(lane.health) }}</span>
              </span>
              <span class="ceo-graph-lane-meta">
                {{ t('ceoboard.graph.flows', { count: lane.workflows.length }) }} ·
                {{ t('ceoboard.graph.volume1h', { count: lane.volume1h }) }}
                <template v-if="lane.alerts">
                  · <span class="ceo-alert-num">{{ t('ceoboard.lane.headlineAlerts', { count: lane.alerts }) }}</span>
                </template>
              </span>
            </button>

            <div class="ceo-graph-wfs">
              <div v-for="wf in lane.workflows" :key="wf.id" class="ceo-graph-wf">
                <button
                  type="button"
                  class="ceo-graph-wf-card ceo-graph-panel"
                  :class="{ 'ceo-graph-working': wf.working }"
                  :aria-expanded="isExpanded(wf)"
                  @click="toggleWf(wf)"
                >
                  <span class="ceo-graph-wf-head">
                    <span class="ceo-dot" :class="wf.health"></span>
                    <span class="ceo-graph-wf-name">{{ wf.name }}</span>
                  </span>
                  <span class="ceo-graph-wf-status" :class="wf.worstHealth">{{ wfConclusion(wf) }}</span>
                  <span class="ceo-graph-wf-meta ceo-mono">
                    {{ t('ceoboard.graph.nodes', { count: wf.nodes.length }) }} ·
                    {{ t('ceoboard.graph.volume1h', { count: wf.volume1h }) }}
                    <template v-if="wf.alerts">
                      · <span class="ceo-alert-num">{{ t('ceoboard.lane.headlineAlerts', { count: wf.alerts }) }}</span>
                    </template>
                  </span>
                </button>
                <div v-if="isExpanded(wf)" class="ceo-graph-nodes">
                  <div
                    v-for="n in wf.nodes"
                    :key="n.id"
                    class="ceo-graph-node"
                    :class="{ 'ceo-graph-working': n.working }"
                    role="button"
                    tabindex="0"
                    :title="n.reason"
                    @click="emit('openNode', wf.id, n.id)"
                    @keydown.enter.prevent="emit('openNode', wf.id, n.id)"
                    @keydown.space.prevent="emit('openNode', wf.id, n.id)"
                  >
                    <span class="ceo-dot" :class="n.health"></span>
                    <span class="ceo-graph-node-name">{{ n.order }}. {{ n.name }}</span>
                    <small class="ceo-graph-node-reason" :class="n.health">{{ friendlyReason(n.reason) }}</small>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
