<template>
  <div>
    <div v-if="loading" class="column items-center q-py-lg">
      <q-spinner color="primary" size="32px" />
    </div>
    <div v-else-if="error" class="text-negative q-pa-md">{{ error }}</div>
    <div v-else-if="!messages.length" class="text-grey-7 q-pa-md">暂无消息记录</div>
    <div v-else class="message-list">
      <div v-for="msg in messages" :key="msg.id" class="message-row" :class="`message-row--${msg.role}`">
        <div class="message-row__avatar">
          <q-icon :name="msg.role === 'user' ? 'person' : 'smart_toy'" size="20px" />
        </div>
        <div class="message-row__body">
          <div class="row items-center q-gutter-sm">
            <span class="text-caption text-weight-bold">{{ roleLabel(msg.role) }}</span>
            <span v-if="msg.model_name" class="text-caption text-grey-6">{{ msg.model_name }}</span>
            <span class="text-caption text-grey-6">{{ formatDate(msg.created_at) }}</span>
            <q-badge v-if="msg.status !== 'ok'" :color="msg.status === 'error' ? 'negative' : 'warning'" outline>{{ msg.status }}</q-badge>
          </div>
          <div class="message-row__content">{{ msg.content_markdown }}</div>
          <div v-if="msg.token_in || msg.token_out" class="text-caption text-grey-6 q-mt-xs">
            IN {{ msg.token_in }} · OUT {{ msg.token_out }}
            <span v-if="msg.latency_ms"> · {{ msg.latency_ms }}ms</span>
          </div>
          <div v-if="msg.error_message" class="text-caption text-negative q-mt-xs">{{ msg.error_message }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import type { Message } from "../../features/chat/types";
import { useSessionStore } from "../../stores/session/index";

const props = defineProps<{ sessionId: string }>();

const sessionStore = useSessionStore();
const messages = ref<Message[]>([]);
const loading = ref(false);
const error = ref("");

function roleLabel(role: string) {
  if (role === "user") return "用户";
  if (role === "assistant") return "助手";
  if (role === "system") return "系统";
  return role;
}

function formatDate(value: string) {
  if (!value) return "—";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

async function loadMessages() {
  loading.value = true;
  error.value = "";
  try {
    messages.value = await sessionStore.fetchMessages(props.sessionId);
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载消息失败";
  } finally {
    loading.value = false;
  }
}

onMounted(loadMessages);
</script>

<style scoped>
.message-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.message-row {
  display: flex;
  gap: 12px;
  padding: 12px 16px;
  border-radius: 12px;
  border: 1px solid rgb(100 116 139 / 15%);
  background: var(--color-on-accent);
}

.message-row--user {
  background: rgb(59 130 246 / 4%);
}

.message-row--assistant {
  background: rgb(16 185 129 / 4%);
}

.message-row__avatar {
  flex: 0 0 auto;
  padding-top: 2px;
  color: var(--color-text-tertiary);
}

.message-row__body {
  flex: 1 1 auto;
  min-width: 0;
}

.message-row__content {
  margin-top: 6px;
  white-space: pre-wrap;
  overflow-wrap: break-word;
  color: var(--color-text-primary);
  font-size: 14px;
  line-height: 1.6;
}

:global(.body--dark) .message-row {
  border-color: rgb(148 163 184 / 18%);
  background: rgb(15 23 42 / 60%);
}

:global(.body--dark) .message-row--user {
  background: rgb(59 130 246 / 8%);
}

:global(.body--dark) .message-row--assistant {
  background: rgb(16 185 129 / 8%);
}
</style>
