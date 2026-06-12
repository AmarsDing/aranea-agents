<template>
  <div class="team-panel">
    <!-- Task Board Section -->
    <TaskBoardSection v-if="panel.taskBoard" :section="panel.taskBoard" />

    <!-- DAG Section -->
    <DagSection v-if="panel.dag" :section="panel.dag" />

    <!-- Team Progress Section -->
    <TeamProgressSection
      v-for="team in panel.teamProgress"
      :key="team.teamId"
      :section="team"
    />
  </div>
</template>

<script setup lang="ts">
import type { TeamPanel as TeamPanelType } from '../../features/chat/activityTimelineTypes';
import TaskBoardSection from './TaskBoardSection.vue';
import DagSection from './DagSection.vue';
import TeamProgressSection from './TeamProgressSection.vue';

defineProps<{
  panel: TeamPanelType;
}>();
</script>

<style lang="sass" scoped>
.team-panel
  background: var(--glass-surface)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))
  border: 1px solid var(--glass-border)
  border-radius: 16px
  padding: 16px
  margin-bottom: 8px
</style>
