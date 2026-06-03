<template>
  <div>
    <div v-if="loading" class="column items-center q-py-lg">
      <q-spinner color="primary" size="32px" />
    </div>
    <div v-else-if="error" class="text-negative q-pa-md">{{ error }}</div>
    <div v-else-if="!participants.length" class="text-grey-7 q-pa-md">暂无参与者记录</div>
    <q-list v-else separator>
      <q-item v-for="row in participants" :key="row.id" class="app-interactive-list-item">
        <q-item-section avatar>
          <q-avatar color="primary" text-color="white" size="36px">
            {{ avatarLabel(row.display_name) }}
          </q-avatar>
        </q-item-section>
        <q-item-section>
          <q-item-label>{{ row.display_name || row.participant_id }}</q-item-label>
          <q-item-label caption class="text-grey-6">
            {{ row.participant_type }} · {{ row.role_in_session || 'member' }}
          </q-item-label>
        </q-item-section>
        <q-item-section side class="text-right">
          <div class="text-caption text-grey-7">{{ row.message_count }} 消息</div>
          <div class="text-caption text-grey-7">IN {{ row.input_tokens }} · OUT {{ row.output_tokens }}</div>
        </q-item-section>
      </q-item>
    </q-list>
  </div>
</template>

<script setup lang="ts">
import { toRef } from 'vue';
import { useSessionParticipantsPanel } from '../../features/session/useSessionParticipantsPanel';

const props = defineProps<{ sessionId: string }>();

const { participants, loading, error } = useSessionParticipantsPanel(toRef(() => props.sessionId));

function avatarLabel(name: string) {
  const trimmed = name.trim();
  if (!trimmed) return '?';
  return trimmed.slice(0, 1).toUpperCase();
}
</script>
