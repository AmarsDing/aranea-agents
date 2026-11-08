<script setup lang="ts">
// 业务线卡片（整合 P2-8，自 ceo-dashboard BizLaneCard.vue 移植）：老板视图
// 的第三层——「我的各条业务线现在怎么样」。默认每条流程一行结论；异常流程
// 自动展开环节；点击环节开抽屉看工程细节。
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { LaneView, NodeView, WorkflowView } from '../../features/ceoboard/types';
import { healthRank, sortWorkflows } from '../../features/ceoboard/healthRank';
import { friendlyReason, healthLabel, laneName, metricName } from '../../features/ceoboard/friendly';

const props = defineProps<{ lane: LaneView }>();
const emit = defineEmits<{ openNode: [wfId: string, nodeId: string] }>();
const { t } = useI18n();

const wfs = computed(() => sortWorkflows(props.lane.workflows));

const laneHealth = computed(() => {
  const hs = props.lane.workflows.map((w) => w.health);
  return hs.slice().sort((a, b) => healthRank(a) - healthRank(b))[0] ?? 'gray';
});

const totalVolume = computed(() => props.lane.workflows.reduce((s, w) => s + (w.volume_1h ?? 0), 0));
const totalAlerts = computed(() => props.lane.workflows.reduce((s, w) => s + (w.alerts ?? 0), 0));

// 展开状态 = 用户手动覆盖 ?? 默认(异常/隐患展开,正常折叠);数据刷新不冲掉用户选择
const overrides = ref<Record<string, boolean>>({});
function isExpanded(wf: WorkflowView): boolean {
  return overrides.value[wf.id] ?? (wf.health === 'red' || wf.health === 'yellow');
}
function toggle(wf: WorkflowView) {
  overrides.value[wf.id] = !isExpanded(wf);
}

// 流程的一句话结论：取最不健康的那个环节的原因
function wfStatus(wf: WorkflowView): { text: string; health: string } {
  const worst = wf.nodes.slice().sort((a, b) => healthRank(a.health) - healthRank(b.health))[0];
  if (!worst) return { text: t('ceoboard.lane.noNodeData'), health: 'gray' };
  if (worst.health === 'green' || worst.health === 'gray') {
    return { text: t('ceoboard.lane.runningOk', { count: wf.nodes.length }), health: worst.health };
  }
  return { text: friendlyReason(worst.reason), health: worst.health };
}

function nodeTitle(n: NodeView): string {
  return [n.reason, n.source?.last_err ? t('ceoboard.wf.tipErr', { msg: n.source.last_err }) : '']
    .filter(Boolean)
    .join('\n');
}
</script>

<template>
  <div class="ceo-biz-card" :class="laneHealth">
    <header class="ceo-biz-head">
      <span class="ceo-biz-name">{{ laneName(lane.biz_line) }}</span>
      <span class="ceo-biz-pill" :class="laneHealth">{{ healthLabel(laneHealth) }}</span>
      <span class="ceo-biz-headline">
        {{ t('ceoboard.lane.headline', { count: lane.workflows.length, volume: totalVolume }) }}
        <template v-if="totalAlerts">
          · <span class="ceo-alert-num">{{ t('ceoboard.lane.headlineAlerts', { count: totalAlerts }) }}</span>
        </template>
      </span>
    </header>

    <div v-if="!lane.workflows.length" class="ceo-empty ceo-biz-empty">{{ t('ceoboard.lane.empty') }}</div>

    <div v-for="wf in wfs" :key="wf.id" class="ceo-biz-wf">
      <div
        class="ceo-biz-wf-row"
        role="button"
        tabindex="0"
        :aria-expanded="isExpanded(wf)"
        @click="toggle(wf)"
        @keydown.enter.prevent="toggle(wf)"
        @keydown.space.prevent="toggle(wf)"
      >
        <span class="ceo-dot" :class="wf.health"></span>
        <span class="ceo-biz-wf-name">{{ wf.name }}</span>
        <span class="ceo-biz-wf-status" :class="wfStatus(wf).health">{{ wfStatus(wf).text }}</span>
        <span class="ceo-biz-wf-meta">
          {{ t('ceoboard.lane.volume', { count: wf.volume_1h })
          }}<template v-if="wf.alerts">
            · <span class="ceo-alert-num ceo-alert-num--bold">{{ t('ceoboard.lane.headlineAlerts', { count: wf.alerts }) }}</span>
          </template>
        </span>
        <span class="ceo-biz-chevron" :class="{ open: isExpanded(wf) }">▸</span>
      </div>
      <div v-if="isExpanded(wf)" class="ceo-biz-nodes">
        <div
          v-for="n in wf.nodes"
          :key="n.id"
          class="ceo-biz-node"
          role="button"
          tabindex="0"
          :title="nodeTitle(n)"
          @click.stop="emit('openNode', wf.id, n.id)"
          @keydown.enter.stop.prevent="emit('openNode', wf.id, n.id)"
        >
          <span class="ceo-dot" :class="n.health"></span>
          <span class="ceo-biz-node-name">{{ n.order }}. {{ n.name }}</span>
          <span class="ceo-biz-node-reason" :class="n.health">{{ friendlyReason(n.reason) }}</span>
          <span v-for="m in n.metrics" :key="m.id" class="ceo-mchip" :class="{ pending: m.pending }" :title="m.id">
            <span class="mk">{{ metricName(m.id, m.note) }}</span>{{ m.display }}
          </span>
        </div>
      </div>
    </div>
  </div>
</template>
