<!-- web/src/components/chat/v2/MemberSessionPanel.vue -->
<template>
  <div class="member-session-panel" :data-agent-key="memberSession.AgentKey">
    <div class="member-header">
      <q-avatar v-if="memberSession.AvatarURL" :src="memberSession.AvatarURL" size="32px" />
      <span class="member-name">{{ memberSession.AgentName }}</span>
      <q-badge v-if="memberSession.Status === 'running'" color="blue" :label="t('chat.v2.statusRunning')" />
      <q-badge v-else-if="memberSession.Status === 'completed'" color="green" :label="t('chat.v2.statusCompleted')" />
      <q-badge v-else-if="memberSession.Status === 'failed'" color="red" :label="t('chat.v2.statusFailed')" />
    </div>
    <div v-if="memberSession.Error" class="member-error">{{ memberSession.Error }}</div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { MemberSession } from '../../../features/chat/v2Types';

// Safe i18n wrapper — falls back to the key when the i18n plugin isn't
// installed (e.g., during unit tests without app.use(i18n)).
function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string) => key };
  }
}

defineProps<{ memberSession: MemberSession }>();
const { t } = useSafeI18n();
</script>
