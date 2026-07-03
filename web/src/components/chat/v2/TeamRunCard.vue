<!-- web/src/components/chat/v2/TeamRunCard.vue -->
<template>
  <div class="team-run-card" :data-team-run-id="teamRun.ID">
    <div class="team-run-header">
      <q-badge :color="statusColor">{{ teamRun.Status }}</q-badge>
    </div>
    <MemberSessionPanel v-for="ms in memberSessions" :key="ms.ID" :member-session="ms" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { TeamRun } from '../../../features/chat/v2Types';
import MemberSessionPanel from './MemberSessionPanel.vue';

const props = defineProps<{ teamRun: TeamRun }>();
const store = useChatActivityStore();
const memberSessions = computed(() => store.getTeamRunMemberSessions(props.teamRun.ID));
const statusColor = computed(
  () =>
    ({
      running: 'blue',
      completed: 'green',
      failed: 'red',
      cancelled: 'grey',
    })[props.teamRun.Status] || 'grey',
);
</script>
