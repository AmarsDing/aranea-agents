<template>
  <div class="chat-diff-viewer" :class="{ 'chat-diff-viewer--dark': isDark }">
    <div class="chat-diff-viewer__header row items-center no-wrap">
      <q-icon name="difference" size="16px" class="chat-diff-viewer__icon" />
      <span class="chat-diff-viewer__file ellipsis">{{ fileName }}</span>
      <span class="chat-diff-viewer__summary text-caption">{{ summary }}</span>
    </div>

    <div class="chat-diff-viewer__hunks">
      <div v-for="(hunk, idx) in hunks" :key="idx" class="chat-diff-viewer__hunk">
        <div v-if="hunk.searchLines.length" class="chat-diff-viewer__section chat-diff-viewer__section--old">
          <div v-for="(line, li) in hunk.searchLines" :key="'o' + li" class="chat-diff-viewer__line chat-diff-viewer__line--old">
            <span class="chat-diff-viewer__marker">-</span>
            <span class="chat-diff-viewer__text">{{ line }}</span>
          </div>
        </div>
        <div v-if="hunk.replaceLines.length" class="chat-diff-viewer__section chat-diff-viewer__section--new">
          <div v-for="(line, li) in hunk.replaceLines" :key="'n' + li" class="chat-diff-viewer__line chat-diff-viewer__line--new">
            <span class="chat-diff-viewer__marker">+</span>
            <span class="chat-diff-viewer__text">{{ line }}</span>
          </div>
        </div>
      </div>
    </div>

    <div v-if="showActions" class="chat-diff-viewer__actions row items-center justify-end q-mt-xs">
      <q-btn
        flat
        dense
        no-caps
        :label="t('chat.reject', '拒绝')"
        color="negative"
        size="11px"
        class="q-mr-xs"
        @click="$emit('reject', { toolName, fileName })"
      />
      <q-btn
        flat
        dense
        no-caps
        :label="t('chat.apply', '应用')"
        color="positive"
        size="11px"
        @click="$emit('apply', { toolName, fileName })"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import type { DiffEditHunk } from "../../features/chat/types";

const props = withDefaults(
  defineProps<{
    fileName: string;
    hunks: DiffEditHunk[];
    toolName: string;
    appliedCount?: number;
    isDark?: boolean;
    showActions?: boolean;
  }>(),
  { appliedCount: 0, isDark: false, showActions: false },
);

defineEmits<{
  apply: [payload: { toolName: string; fileName: string }];
  reject: [payload: { toolName: string; fileName: string }];
}>();

const { t } = useI18n();

const summary = computed(() => {
  const total = props.hunks.length;
  if (props.appliedCount > 0) {
    return t("chat.diffAppliedCount", `${props.appliedCount} applied`);
  }
  return t("chat.diffHunkCount", `${total} hunk(s)`);
});
</script>

<style scoped>
.chat-diff-viewer {
  border-radius: 8px;
  overflow: hidden;
  border: 1px solid var(--glass-border);
  background: var(--glass-surface);
}

.chat-diff-viewer__header {
  padding: 6px 10px;
  gap: 6px;
  border-bottom: 1px solid var(--glass-border);
  background: color-mix(in srgb, var(--color-text-primary) 4%, transparent);
}

.chat-diff-viewer__icon {
  color: var(--color-accent);
  flex-shrink: 0;
}

.chat-diff-viewer__file {
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-primary);
  font-family: monospace;
  min-width: 0;
}

.chat-diff-viewer__summary {
  flex-shrink: 0;
  color: var(--color-text-secondary);
  font-size: 11px;
}

.chat-diff-viewer__hunks {
  max-height: 320px;
  overflow-y: auto;
  font-family: monospace;
  font-size: 12px;
  line-height: 1.5;
}

.chat-diff-viewer__hunk + .chat-diff-viewer__hunk {
  border-top: 1px dashed var(--glass-border);
}

.chat-diff-viewer__section--old {
  background: var(--color-diff-removed-bg);
}

.chat-diff-viewer__section--new {
  background: var(--color-diff-added-bg);
}

.chat-diff-viewer__line {
  display: flex;
  padding: 0 10px;
  min-height: 20px;
}

.chat-diff-viewer__line--old {
  color: var(--color-text-primary);
}

.chat-diff-viewer__line--new {
  color: var(--color-text-primary);
}

.chat-diff-viewer__marker {
  flex-shrink: 0;
  width: 14px;
  font-weight: 700;
  user-select: none;
}

.chat-diff-viewer__line--old .chat-diff-viewer__marker {
  color: var(--color-diff-removed);
}

.chat-diff-viewer__line--new .chat-diff-viewer__marker {
  color: var(--color-diff-added);
}

.chat-diff-viewer__text {
  white-space: pre-wrap;
  word-break: break-all;
  min-width: 0;
}

.chat-diff-viewer__actions {
  padding: 4px 10px;
  border-top: 1px solid var(--glass-border);
}
</style>
