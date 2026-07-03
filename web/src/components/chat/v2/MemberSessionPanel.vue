<!-- web/src/components/chat/v2/MemberSessionPanel.vue -->
<template>
  <div class="member-session-panel" :data-agent-key="memberSession.AgentKey">
    <div class="member-header">
      <q-avatar v-if="memberSession.AvatarURL" :src="memberSession.AvatarURL" size="32px" />
      <span class="member-name">{{ memberSession.AgentName }}</span>
      <q-badge v-if="memberSession.Status === 'running'" color="blue" label="执行中" />
      <q-badge v-else-if="memberSession.Status === 'completed'" color="green" label="完成" />
      <q-badge v-else-if="memberSession.Status === 'failed'" color="red" label="失败" />
    </div>
    <div v-if="memberSession.Error" class="member-error">{{ memberSession.Error }}</div>
  </div>
</template>

<script setup lang="ts">
import type { MemberSession } from '../../../features/chat/v2Types';
defineProps<{ memberSession: MemberSession }>();
</script>
