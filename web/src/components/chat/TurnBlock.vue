<template>
  <!-- Collapsed summary view -->
  <div v-if="collapsed && block.isCompleted" class="turn-block turn-block--collapsed" @click="emit('toggle-collapse')">
    <div class="row items-center no-wrap q-gutter-xs">
      <q-icon :name="collapsedIcon" size="16px" :color="collapsedIconColor" />
      <span class="text-caption ellipsis">{{ collapsedSummary }}</span>
      <q-space />
      <span v-if="collapsedDuration" class="text-caption text-grey-7">{{ collapsedDuration }}</span>
      <q-icon name="expand_more" size="14px" color="grey-6" />
    </div>
  </div>
  <!-- Full content view -->
  <article v-else class="turn-block" :class="{ 'turn-block--focused': focused }" :data-turn-id="block.turnId">
    <div v-if="turnSourceLabel" class="turn-block__channel-bar text-caption" :aria-label="turnSourceLabel">
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
      :reasoning-sidebar-open="reasoningSidebarOpen"
      @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
      @retry="(id) => emit('retry', id)"
      @dismiss-failed="(id) => emit('dismiss-failed', id)"
      @pin-reasoning="(id) => emit('pin-reasoning', id)"
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
        :reasoning-sidebar-open="reasoningSidebarOpen"
        @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
        @feedback="(p) => emit('feedback', p)"
        @regenerate="(msg) => emit('regenerate', msg)"
        @pin-reasoning="(id) => emit('pin-reasoning', id)"
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
        :reasoning-sidebar-open="reasoningSidebarOpen"
        @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
        @retry="(id) => emit('retry', id)"
        @dismiss-failed="(id) => emit('dismiss-failed', id)"
        @regenerate="(msg) => emit('regenerate', msg)"
        @pin-reasoning="(id) => emit('pin-reasoning', id)"
      />
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import ChatMessageRow from './ChatMessageRow.vue';
import ToolStrip from './ToolStrip.vue';
import type { TurnBlockGroup } from '../../features/chat/groupMessagesByTurn';
import { filterToolsForToolStrip, toolStripSummary } from '../../features/chat/groupMessagesByTurn';
import { toolEventFromMessage } from '../../features/chat/envelopeToolCall';
import {
  messageSourceChipFallback,
  messageSourceChipKey,
  messageSourceFromMessage,
} from '../../features/chat/messageSourceMeta';
import type { A2UIUserActionPayload } from '../../features/chat/a2uiUserAction';
import type { Message, ReactToolLinkIndex } from '../../features/chat/types';

const props = withDefaults(
  defineProps<{
    block: TurnBlockGroup;
    allMessages: Message[];
    isDark: boolean;
    isTeamSession?: boolean;
    plannerKind?: string;
    reactToolLinkIndex: ReactToolLinkIndex;
    focused?: boolean;
    reasoningSidebarOpen?: boolean;
    collapsed?: boolean;
  }>(),
  { collapsed: false },
);

const emit = defineEmits<{
  'a2ui-user-action': [payload: A2UIUserActionPayload];
  feedback: [payload: { messageId: string; rating: 'positive' | 'negative' }];
  regenerate: [message: Message];
  retry: [messageId: string];
  'dismiss-failed': [messageId: string];
  'pin-reasoning': [messageId: string];
  'toggle-collapse': [];
}>();

const { t } = useI18n();

const turnSourceLabel = computed(() => {
  const meta = messageSourceFromMessage(props.block.user ?? null);
  if (!meta) return '';
  const key = messageSourceChipKey(meta);
  return key ? t(key, messageSourceChipFallback(meta)) : messageSourceChipFallback(meta);
});

const visibleTools = computed(() => filterToolsForToolStrip(props.block.tools, props.reactToolLinkIndex));

const collapsedSummary = computed(() => {
  const tools = props.block.tools;
  const summary = toolStripSummary(tools);
  if (tools.length === 0) {
    return props.block.assistant?.content_markdown?.slice(0, 60) || '已完成';
  }
  if (tools.length === 1) {
    const ev = toolEventFromMessage(tools[0]!);
    const name = ev?.display_label || ev?.tool_name || '工具';
    return `${name} · ${summary.failed > 0 ? '部分失败' : '完成'}`;
  }
  return `${tools.length} tools · ${summary.failed > 0 ? `${summary.failed} failed` : '完成'}`;
});

const collapsedIcon = computed(() => {
  const summary = toolStripSummary(props.block.tools);
  if (summary.failed > 0) return 'error';
  return 'check_circle';
});

const collapsedIconColor = computed(() => {
  const summary = toolStripSummary(props.block.tools);
  if (summary.failed > 0) return 'negative';
  return 'positive';
});

const collapsedDuration = computed(() => {
  const summary = toolStripSummary(props.block.tools);
  if (summary.totalMs > 0) {
    const sec = Math.round(summary.totalMs / 1000);
    return `${sec}s`;
  }
  return '';
});
</script>

<style scoped lang="sass">
.turn-block
  margin-bottom: var(--space-2)
  padding: var(--space-2) var(--space-2) var(--space-1)
  border-radius: 14px
  background: color-mix(in srgb, var(--glass-surface) 35%, transparent)
  border: 1px solid color-mix(in srgb, var(--glass-border) 50%, transparent)
  transition: box-shadow 0.25s ease, border-color 0.25s ease, background 0.2s ease

.turn-block--focused
  border-color: color-mix(in srgb, var(--color-accent) 55%, var(--glass-border))
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--color-accent) 25%, transparent)
  background: color-mix(in srgb, var(--glass-surface) 50%, transparent)

.turn-block__channel-bar
  margin: calc(-1 * var(--space-1)) 0 var(--space-2)
  padding: var(--space-1) var(--space-2)
  border-radius: 8px
  background: color-mix(in srgb, var(--color-accent) 10%, transparent)
  color: var(--color-text-secondary)
  font-size: 11.5px
  font-weight: 600
  letter-spacing: 0.02em

.turn-block__response
  position: relative
  padding-top: var(--space-1)
  margin-top: var(--space-1)
  border-top: 1px solid color-mix(in srgb, var(--glass-border) 35%, transparent)

.turn-block--collapsed
  cursor: pointer
  padding: var(--space-1) var(--space-2)
  margin-bottom: var(--space-1)
  border-radius: 8px
  background: color-mix(in srgb, var(--glass-surface) 25%, transparent)
  border: 1px solid color-mix(in srgb, var(--glass-border) 30%, transparent)
  transition: background 0.15s ease

  &:hover
    background: color-mix(in srgb, var(--glass-surface) 45%, transparent)
</style>
