<template>
  <div v-if="messages.length" class="chat-pending-list">
    <div class="chat-pending-label">{{ t('chat.pendingQueue') }}</div>
    <div v-for="pm in messages" :key="pm.id" class="chat-pending-item">
      <div v-if="editingId === pm.id" class="chat-pending-item__edit">
        <q-input
          v-model="editingContent"
          dense
          outlined
          autogrow
          class="chat-pending-item__edit-input"
          :dark="isDark"
          @keydown.enter.prevent="confirmEdit(pm.id)"
          @keydown.escape.prevent="cancelEdit"
        />
        <q-btn
          dense
          flat
          round
          size="sm"
          icon="check"
          color="positive"
          class="chat-pending-item__edit-confirm"
          :aria-label="t('chat.confirmEdit')"
          @click="confirmEdit(pm.id)"
        />
        <q-btn
          dense
          flat
          round
          size="sm"
          icon="close"
          color="negative"
          class="chat-pending-item__edit-cancel"
          :aria-label="t('chat.cancelEdit')"
          @click="cancelEdit"
        />
      </div>
      <template v-else>
        <div class="chat-pending-item__content ellipsis">{{ pm.content }}</div>
        <div class="chat-pending-item__meta">
          <span class="chat-pending-item__status">{{ pm.status }}</span>
          <span class="chat-pending-item__time">{{ formatStamp(pm.created_at) }}</span>
          <q-btn
            dense
            flat
            round
            size="sm"
            icon="edit"
            color="primary"
            class="chat-pending-item__edit-btn"
            :aria-label="t('chat.editPending')"
            @click="startEdit(pm)"
          />
          <q-btn
            dense
            flat
            round
            size="sm"
            icon="cancel"
            color="negative"
            class="chat-pending-item__cancel"
            :aria-label="t('chat.cancelPending')"
            @click="$emit('cancel-pending', pm.id)"
          />
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';

export type PendingMessageRow = {
  id: string;
  content: string;
  status: string;
  created_at: string;
};

defineProps<{
  messages: PendingMessageRow[];
  isDark?: boolean;
}>();

const emit = defineEmits<{
  'cancel-pending': [pendingId: string];
  'update-pending': [pendingId: string, content: string];
}>();

const { t } = useI18n();
const editingId = ref('');
const editingContent = ref('');

function formatStamp(iso: string) {
  if (!iso) return '';
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

function startEdit(pm: PendingMessageRow) {
  editingId.value = pm.id;
  editingContent.value = pm.content;
}

function confirmEdit(pendingId: string) {
  const content = editingContent.value.trim();
  if (!content) return;
  emit('update-pending', pendingId, content);
  editingId.value = '';
  editingContent.value = '';
}

function cancelEdit() {
  editingId.value = '';
  editingContent.value = '';
}
</script>
