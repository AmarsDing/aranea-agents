<template>
  <div class="chat-a2ui-preview q-mb-sm">
    <div class="row items-center justify-between q-mb-xs">
      <span class="text-caption text-weight-medium">A2UI 界面</span>
      <q-btn
        v-if="hasErrors"
        flat
        dense
        size="sm"
        :label="showRaw ? '隐藏原始 JSONL' : '查看原始 JSONL'"
        @click="showRaw = !showRaw"
      />
    </div>

    <ChatA2UISurface
      v-if="surface.ready"
      :surface="surface"
      @user-action="(p) => emit('user-action', p)"
    />

    <div v-if="hasErrors || (showRaw && lines.length)" class="q-mt-sm">
      <div
        v-for="row in lines"
        :key="row.lineNumber"
        class="chat-a2ui-line"
        :class="{ 'chat-a2ui-line--err': !row.ok }"
      >
        <template v-if="showRaw || !row.ok">
          <template v-if="row.ok">
            <q-chip dense size="sm" color="grey-7" text-color="white" class="q-mr-sm">{{ row.key }}</q-chip>
            <pre class="chat-a2ui-line__json">{{ formatPayload(row.payload) }}</pre>
          </template>
          <template v-else>
            <q-icon name="error_outline" color="negative" size="16px" class="q-mr-xs" />
            <span class="text-caption text-negative">{{ row.error }}</span>
            <pre class="chat-a2ui-line__json q-mt-xs">{{ row.raw }}</pre>
          </template>
        </template>
      </div>
    </div>

    <q-banner v-if="!surface.ready && !hasErrors" rounded dense class="settings-info-banner">
      等待 beginRendering / surfaceUpdate…
    </q-banner>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import type { A2UIParseLine } from "../../features/chat/a2uiParse";
import { reduceA2UISurface } from "../../features/chat/a2uiSurfaceState";
import type { A2UIUserActionPayload } from "../../features/chat/a2uiUserAction";
import ChatA2UISurface from "./ChatA2UISurface.vue";

const props = defineProps<{
  lines: A2UIParseLine[];
}>();

const emit = defineEmits<{
  "user-action": [payload: A2UIUserActionPayload];
}>();

const showRaw = ref(false);

const surface = computed(() => reduceA2UISurface(props.lines));
const hasErrors = computed(() => props.lines.some((l) => !l.ok));

function formatPayload(payload: Record<string, unknown>): string {
  try {
    return JSON.stringify(payload, null, 2);
  } catch {
    return String(payload);
  }
}
</script>

<style scoped>
.chat-a2ui-preview {
  padding: 10px 12px;
  border-radius: 14px;
  border: 1px solid var(--glass-border);
  background: var(--glass-elevated);
}

.chat-a2ui-line {
  margin-bottom: 8px;
}

.chat-a2ui-line--err {
  border-left: 3px solid var(--q-negative);
  padding-left: 8px;
}

.chat-a2ui-line__json {
  margin: 4px 0 0;
  padding: 8px;
  border-radius: 8px;
  background: var(--glass-surface);
  font-size: 12px;
  overflow-x: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
