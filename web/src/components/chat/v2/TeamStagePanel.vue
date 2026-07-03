<!-- web/src/components/chat/v2/TeamStagePanel.vue -->
<template>
  <div class="team-stage-panel" :data-team-stage-id="teamStage.ID">
    <div class="team-stage-header">
      <span>Team: {{ teamStage.TeamID }}</span>
      <q-badge :color="stageColor">{{ teamStage.Stage }}</q-badge>
    </div>
    <div class="team-members">
      <span v-for="m in teamStage.Members" :key="m.AgentKey" class="member-chip">
        {{ m.AgentName }}
      </span>
    </div>
    <TeamRunCard v-for="tr in teamRuns" :key="tr.ID" :team-run="tr" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { TeamStage } from '../../../features/chat/v2Types';
import TeamRunCard from './TeamRunCard.vue';

const props = defineProps<{ teamStage: TeamStage }>();
const store = useChatActivityStore();
const teamRuns = computed(() => store.getTeamStageTeamRuns(props.teamStage.ID));
const stageColor = computed(() => ({
  assembled: 'grey', planning: 'orange', executing: 'blue',
  completed: 'green', failed: 'red',
}[props.teamStage.Stage] || 'grey'));
</script>
