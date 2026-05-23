<template>
  <article class="turn-block" :data-turn-id="block.turnId">
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
    />
    <ToolStrip
      v-if="block.tools.length"
      :tools="block.tools"
      :is-dark="isDark"
      :is-team-session="isTeamSession"
      :planner-kind="plannerKind"
      :react-tool-link-index="reactToolLinkIndex"
      @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
    />
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
    />
  </article>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import ChatMessageRow from "./ChatMessageRow.vue";
import ToolStrip from "./ToolStrip.vue";
import type { TurnBlockGroup } from "../../features/chat/groupMessagesByTurn";
import {
  messageSourceChipFallback,
  messageSourceChipKey,
  parseMessageSourceMeta,
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
}>();

const emit = defineEmits<{
  "a2ui-user-action": [payload: A2UIUserActionPayload];
}>();

const { t } = useI18n();

const turnSourceLabel = computed(() => {
  const meta = parseMessageSourceMeta(props.block.user?.options_json);
  if (!meta) return "";
  const key = messageSourceChipKey(meta);
  return key ? t(key, messageSourceChipFallback(meta)) : messageSourceChipFallback(meta);
});
</script>

<style scoped>
.turn-block {
  margin-bottom: 12px;
  padding: 12px 14px;
  border-radius: 14px;
  background: var(--app-glass-soft, rgba(255, 255, 255, 0.55));
  border: 1px solid var(--app-border-subtle, rgba(0, 0, 0, 0.06));
}

.turn-block__channel-bar {
  margin: -4px 0 8px;
  padding: 4px 8px;
  border-radius: 8px;
  background: var(--app-info-soft, rgba(33, 150, 243, 0.12));
  color: var(--app-text-secondary, rgba(0, 0, 0, 0.65));
}
</style>
