<script setup lang="ts">
// 泳道分组（整合 P2-8，自 ceo-dashboard LaneGroup.vue 移植）。
// oncall=true：值班视图——红黄工作流展开，绿/灰工作流折叠为一行（同一份数据，仅布局不同）。
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { LaneView, WorkflowView } from '../../features/ceoboard/types';
import { laneCollapsible } from '../../features/ceoboard/healthRank';
import { laneShortName } from '../../features/ceoboard/friendly';
import CeoWorkflowCard from './CeoWorkflowCard.vue';

const props = defineProps<{ lane: LaneView; oncall?: boolean }>();
const emit = defineEmits<{ openNode: [wfId: string, nodeId: string] }>();
const { t } = useI18n();

// 折叠规则(§15.1)：仅绿色/灰色组可折叠；红黄组永不折叠（后端 collapsible 已保证，前端再守一道）
const folded = ref(false);
function toggle() {
  if (props.lane.collapsible && laneCollapsible(props.lane.workflows.map((w) => w.health))) folded.value = !folded.value;
}

function countBy(ws: WorkflowView[], h: string): number {
  return ws.filter((w) => w.health === h).length;
}

// 值班视图拆分：异常(红/黄)恒展开；健康(绿/灰)收进一行摘要
const hot = computed(() => props.lane.workflows.filter((w) => w.health === 'red' || w.health === 'yellow'));
const calm = computed(() => props.lane.workflows.filter((w) => w.health === 'green' || w.health === 'gray'));
</script>

<template>
  <div class="ceo-lane">
    <div class="ceo-lane-head">
      <span
        v-if="lane.collapsible"
        class="ceo-fold"
        role="button"
        tabindex="0"
        @click="toggle"
        @keydown.enter.prevent="toggle"
        @keydown.space.prevent="toggle"
      >
        {{ folded ? t('ceoboard.lane.unfold') : t('ceoboard.lane.fold') }}
      </span>
      <span v-else class="ceo-fold" :title="t('ceoboard.lane.neverFoldTip')">◆</span>
      <span class="ceo-lane-name">{{ laneShortName(lane.biz_line) }}</span>
      <span class="ceo-dim ceo-lane-counts">
        {{
          t('ceoboard.lane.counts', {
            total: lane.workflows.length,
            red: countBy(lane.workflows, 'red'),
            yellow: countBy(lane.workflows, 'yellow'),
            green: countBy(lane.workflows, 'green'),
            gray: countBy(lane.workflows, 'gray'),
          })
        }}
      </span>
    </div>
    <template v-if="!folded">
      <template v-if="oncall">
        <CeoWorkflowCard v-for="wf in hot" :key="wf.id" :wf="wf" @open-node="(n: string) => emit('openNode', wf.id, n)" />
        <div v-if="calm.length" class="ceo-calm-row">
          <span class="ceo-dot green"></span>
          <span class="ceo-dim">{{ t('ceoboard.lane.calm', { count: calm.length }) }}</span>
          <span
            v-for="wf in calm"
            :key="wf.id"
            class="ceo-calm-item"
            role="button"
            tabindex="0"
            @click="emit('openNode', wf.id, wf.nodes[0]?.id ?? '')"
            @keydown.enter.prevent="emit('openNode', wf.id, wf.nodes[0]?.id ?? '')"
          >
            {{ wf.name }}<span class="ceo-dim ceo-mono">({{ wf.volume_1h }})</span>
          </span>
        </div>
        <div v-if="!hot.length && !calm.length" class="ceo-empty">{{ t('ceoboard.lane.calmEmpty') }}</div>
      </template>
      <template v-else>
        <CeoWorkflowCard
          v-for="wf in lane.workflows"
          :key="wf.id"
          :wf="wf"
          @open-node="(n: string) => emit('openNode', wf.id, n)"
        />
      </template>
    </template>
  </div>
</template>
