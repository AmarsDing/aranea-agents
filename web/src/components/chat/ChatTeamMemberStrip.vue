<!--
  Team 会话成员条：展示当前 turn 内各子 Agent 流式状态。
-->
<template>
  <div v-if="members.length" class="team-member-strip row q-gutter-xs q-px-md q-pb-sm">
    <q-chip
      v-for="member in members"
      :key="member.key"
      dense
      square
      :color="member.streaming ? 'primary' : 'grey-3'"
      :text-color="member.streaming ? 'white' : 'dark'"
      :icon="member.streaming ? 'sync' : 'smart_toy'"
    >
      {{ member.label }}
      <q-spinner-dots v-if="member.streaming" size="14px" class="q-ml-xs" />
    </q-chip>
  </div>
</template>

<script setup lang="ts">
export type TeamMemberLane = {
  key: string;
  label: string;
  streaming: boolean;
};

defineProps<{
  members: TeamMemberLane[];
}>();
</script>

<style scoped>
.team-member-strip {
  flex-wrap: wrap;
  align-items: center;
}
</style>
