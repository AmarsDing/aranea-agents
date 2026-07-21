<template>
  <div>
    <div v-if="loading" class="column items-center q-py-lg">
      <q-spinner color="primary" size="32px" />
    </div>
    <div v-else-if="error" class="text-negative q-pa-md">{{ error }}</div>
    <div v-else-if="!messages.length" class="text-grey-7 q-pa-md">暂无消息记录</div>
    <div v-else class="session-message-list">
      <div
        v-for="msg in messages"
        :key="msg.id"
        class="session-message-row"
        :class="`session-message-row--${msg.role}`"
      >
        <div class="session-message-row__avatar">
          <q-icon :name="msg.role === 'user' ? 'person' : 'smart_toy'" size="20px" />
        </div>
        <div class="session-message-row__body">
          <div class="row items-center q-gutter-sm">
            <span class="text-caption text-weight-bold">{{ roleLabel(msg.role) }}</span>
            <span v-if="msg.model_name" class="text-caption text-grey-6">{{ msg.model_name }}</span>
            <span class="text-caption text-grey-6">{{ formatSessionDate(msg.created_at) }}</span>
            <q-badge v-if="msg.status !== 'ok'" :color="msg.status === 'error' ? 'negative' : 'warning'" outline>{{
              statusLabel(msg.status)
            }}</q-badge>
          </div>
          <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->
          <div class="session-message-row__content" v-html="renderMarkdown(msg.content_markdown)"></div>
          <div v-if="msg.token_in || msg.token_out" class="text-caption text-grey-6 q-mt-xs">
            {{ t('sessionDetail.tokenIn') }} {{ msg.token_in }} · {{ t('sessionDetail.tokenOut') }} {{ msg.token_out }}
            <span v-if="msg.latency_ms"> · {{ msg.latency_ms }}ms</span>
          </div>
          <div v-if="msg.error_message" class="text-caption text-negative q-mt-xs">{{ msg.error_message }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { toRef } from 'vue';
import { useI18n } from 'vue-i18n';
import { useSessionMessagesPanel } from '../../features/session/useSessionMessagesPanel';
import { renderChatMarkdown } from '../../features/chat/chatMessageMarkdown';
import { formatSessionDate } from './sessionUi';

const { t } = useI18n();

const props = defineProps<{ sessionId: string }>();

const { messages, loading, error } = useSessionMessagesPanel(toRef(() => props.sessionId));

function renderMarkdown(content: string) {
  return renderChatMarkdown(content || '');
}

function statusLabel(status: string) {
  const key = `sessionDetail.messageStatus.${status}`;
  const translated = t(key);
  return translated !== key ? translated : status;
}

function roleLabel(role: string) {
  if (role === 'user') return '用户';
  if (role === 'assistant') return '助手';
  if (role === 'system') return '系统';
  return role;
}
</script>
