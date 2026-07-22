<!-- web/src/components/chat/v2/TeamStagePanel.vue
  - 2026-07-05 移除 team-stage-header（团队名/状态/成员已在 TeamRunCard 头部与中部显示，重复无意义）
  - 保留容器与高亮（GraphNode 点击定位仍需 data-team-stage-id 与 activity-locate-highlight）
-->
<template>
  <div
    class="team-stage-panel"
    :class="{ 'activity-locate-highlight': isHighlighted }"
    :data-team-stage-id="teamStage.ID"
    :data-team-id="teamStage.TeamID"
  >
    <TeamRunCard
      v-for="tr in teamRuns"
      :key="tr.ID"
      :team-run="tr"
      @pause-agent="(sid) => $emit('pause-agent', sid)"
      @inject-agent="(p) => $emit('inject-agent', p)"
      @retry-team="(teamId) => $emit('retry-team', teamId)"
      @expand="(ids) => $emit('expand', ids)"
      @confirm-step="(p) => $emit('confirm-step', p)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onUnmounted } from 'vue';
import { useActivityQueries } from '../../../features/chat/composables/useActivityQueries';
import { useLocateTeamStage } from '../../../features/chat/composables/useLocateTeamStage';
import type { TeamStage } from '../../../features/chat/v2Types';
import type { ConfirmStepPayload } from '../../../features/chat/types';
import TeamRunCard from './TeamRunCard.vue';

const props = defineProps<{ teamStage: TeamStage }>();
defineEmits<{
  'pause-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
  'retry-team': [teamId: string];
  expand: [sessionIds: string[]];
  'confirm-step': [payload: ConfirmStepPayload];
}>();
const store = useActivityQueries();
const teamRuns = computed(() => store.getTeamStageTeamRuns(props.teamStage.ID));

// P1 #5: GraphNode 点击 → 高亮对应 TeamStagePanel。
// 监听 command 对象（每次 locate() 创建新对象引用，确保重复点击同一节点时 watch 仍触发）。
// 命令匹配当前 teamStage.ID 时高亮 3 秒，然后自动清除。
const { locateTeamStageCommand } = useLocateTeamStage();
const isHighlighted = ref(false);
let highlightTimer: ReturnType<typeof setTimeout> | null = null;
watch(
  () => locateTeamStageCommand.value,
  (cmd) => {
    if (cmd?.teamStageId === props.teamStage.ID) {
      isHighlighted.value = true;
      if (highlightTimer) clearTimeout(highlightTimer);
      highlightTimer = setTimeout(() => {
        isHighlighted.value = false;
        highlightTimer = null;
      }, 3000);
    }
  },
);
onUnmounted(() => {
  if (highlightTimer) clearTimeout(highlightTimer);
});
</script>

<style lang="sass" scoped>
// 2026-07-22 左边线体系：纯语义容器，无边框/背景（保留 data-team-stage-id 与定位高亮）
.team-stage-panel
  margin: 8px 0
</style>
