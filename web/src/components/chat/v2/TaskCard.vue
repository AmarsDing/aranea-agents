<!-- web/src/components/chat/v2/TaskCard.vue -->
<template>
  <div class="task-card" :data-task-id="task.ID">
    <div class="task-user-message">{{ task.UserMessage }}</div>
    <div v-if="task.Status === 'running'" class="task-status">处理中...</div>
    <TurnList :turns="turns" />
    <TeamStagePanel v-for="ts in teamStages" :key="ts.ID" :team-stage="ts" />
    <PlanBoardCard v-for="pb in planBoards" :key="pb.ID" :plan-board="pb" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { Task } from '../../../features/chat/v2Types';
import TurnList from './TurnList.vue';
import TeamStagePanel from './TeamStagePanel.vue';
import PlanBoardCard from './PlanBoardCard.vue';

const props = defineProps<{ task: Task }>();
const store = useChatActivityStore();
const turns = computed(() => store.getTaskTurns(props.task.ID));
const teamStages = computed(() => store.getTaskTeamStages(props.task.ID));
const planBoards = computed(() => store.getTaskPlanBoards(props.task.ID));
</script>
