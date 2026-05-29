<template>
  <article
    class="turn-block"
    :class="{ 'turn-block--focused': focused }"
    :data-turn-id="block.turnId"
  >
    <div
      v-if="turnSourceLabel"
      class="turn-block__channel-bar text-caption"
      :aria-label="turnSourceLabel"
    >
      {{ turnSourceLabel }}
    </div>
    <ChatMessageRow
      v-if="block.user"
      :message="block.user"
      :index="0"
      :messages="allMessages"
      :is-dark="isDark"
      :is-team-session="isTeamSession"
      :planner-kind="plannerKind"
      :react-tool-link-index="reactToolLinkIndex"
      @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
      @retry="(id) => emit('retry', id)"
      @dismiss-failed="(id) => emit('dismiss-failed', id)"
    />
    <ToolStrip
      v-if="visibleTools.length"
      :tools="visibleTools"
      :is-dark="isDark"
      :is-team-session="isTeamSession"
      :planner-kind="plannerKind"
      :react-tool-link-index="reactToolLinkIndex"
      @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
    />
    <div v-if="block.assistant || block.members.length" class="turn-block__response">
      <ChatMessageRow
        v-if="block.assistant"
        :message="block.assistant"
        :index="1"
        :messages="allMessages"
        :is-dark="isDark"
        :is-team-session="isTeamSession"
        :planner-kind="plannerKind"
        :react-tool-link-index="reactToolLinkIndex"
        @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
        @feedback="(p) => emit('feedback', p)"
        @regenerate="(msg) => emit('regenerate', msg)"
      />
      <ChatMessageRow
        v-for="(member, mIdx) in block.members"
        :key="member.id"
        :message="member"
        :index="mIdx + 2"
        :messages="allMessages"
        :is-dark="isDark"
        :is-team-session="isTeamSession"
        :planner-kind="plannerKind"
        :react-tool-link-index="reactToolLinkIndex"
        @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
        @retry="(id) => emit('retry', id)"
        @dismiss-failed="(id) => emit('dismiss-failed', id)"
        @regenerate="(msg) => emit('regenerate', msg)"
      />
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import ChatMessageRow from "./ChatMessageRow.vue";
import ToolStrip from "./ToolStrip.vue";
import type { TurnBlockGroup } from "../../features/chat/groupMessagesByTurn";
import { filterToolsForToolStrip } from "../../features/chat/groupMessagesByTurn";
import {
  messageSourceChipFallback,
  messageSourceChipKey,
  messageSourceFromMessage,
} from "../../features/chat/messageSourceMeta";
import type { A2UIUserActionPayload } from "../../features/chat/a2uiUserAction";
import type { Message, ReactToolLinkIndex } from "../../features/chat/types";

const props = defineProps<{
  block: TurnBlockGroup;
  allMessages: Message[];
  isDark: boolean;
  isTeamSession?: boolean;
  plannerKind?: string;
  reactToolLinkIndex: ReactToolLinkIndex;
  focused?: boolean;
}>();

const emit = defineEmits<{
  "a2ui-user-action": [payload: A2UIUserActionPayload];
  feedback: [payload: { messageId: string; rating: "positive" | "negative" }];
  regenerate: [message: Message];
  retry: [messageId: string];
  "dismiss-failed": [messageId: string];
}>();

const { t } = useI18n();

const turnSourceLabel = computed(() => {
  const meta = messageSourceFromMessage(props.block.user ?? null);
  if (!meta) return "";
  const key = messageSourceChipKey(meta);
  return key ? t(key, messageSourceChipFallback(meta)) : messageSourceChipFallback(meta);
});

const visibleTools = computed(() =>
  filterToolsForToolStrip(props.block.tools, props.reactToolLinkIndex)
);
</script>

<style scoped>
.turn-block {
  margin-bottom: var(--space-3);
  padding: var(--space-3) var(--space-3);
  border-radius: 14px;
  background: var(--glass-surface);
  border: 1px solid var(--glass-border);
  transition: box-shadow 0.25s ease, border-color 0.25s ease;
}

.turn-block--focused {
  border-color: var(--color-accent);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-accent) 35%, transparent);
}

.turn-block__channel-bar {
  margin: calc(-1 * var(--space-1)) 0 var(--space-2);
  padding: var(--space-1) var(--space-2);
  border-radius: 8px;
  background: color-mix(in srgb, var(--color-accent) 12%, transparent);
  color: var(--color-text-secondary);
}

.turn-block__response {
  position: relative;
  padding-top: var(--space-1);
  margin-top: var(--space-1);
  border-top: 1px solid color-mix(in srgb, var(--glass-border) 50%, transparent);
}
</style>
