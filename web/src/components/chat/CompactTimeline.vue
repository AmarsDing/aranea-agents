<!--
  CompactTimeline：非 ReAct 模式的紧凑时间线展示。

  替换原 `interleavedRounds` 1:1 配对渲染。
  - 不切分段落
  - 不配对 thinking/reply
  - 不均分工具

  @see docs/reports/2026-06-10-proposal-chat-compact-timeline.md
-->
<template>
  <div v-if="nodes.length" class="compact-timeline">
    <div
      v-for="node in nodes"
      :key="nodeKey(node)"
      class="compact-timeline__node"
      :class="nodeClasses(node)"
    >
      <!-- Thinking node -->
      <template v-if="node.kind === 'thinking'">
        <div class="compact-timeline__node-header">
          <q-icon name="psychology_alt" size="13px" color="accent" />
          <span class="compact-timeline__node-label" :style="{ color: 'var(--color-accent)' }">
            {{ t('chat.turn.block.thinkingLabel', '思考') }}
          </span>
          <q-space />
          <span class="compact-timeline__node-meta">·</span>
        </div>
        <ChatReasoningPeek
          :message-id="node.messageId"
          :reasoning="node.text"
          :is-dark="isDark"
          :streaming="false"
          :thinking-only="false"
        />
      </template>

      <!-- Tool node -->
      <template v-else-if="node.kind === 'tool'">
        <ChatExecutionCard
          :event="node.event"
          :initial-collapsed="isToolEventCompleted(node.event)"
          @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
        />
      </template>

      <!-- Reply node -->
      <template v-else-if="node.kind === 'reply'">
        <div class="compact-timeline__node-header">
          <q-icon
            :name="replyIcon(node.status)"
            :color="replyColor(node.status)"
            size="13px"
          />
          <span class="compact-timeline__node-label" :class="`text-${replyColor(node.status)}`">
            {{ t('chat.turn.block.resultLabel', '回复') }}
          </span>
          <span v-if="node.status === 'streaming'" class="compact-timeline__node-meta">· {{ t('chat.compactTimeline.generating') }}</span>
          <span v-else-if="node.status === 'failed'" class="compact-timeline__node-meta">· {{ t('chat.turn.block.failed') }}</span>
          <span v-else-if="node.status === 'cancelled'" class="compact-timeline__node-meta">· {{ t('chat.turn.block.cancelled') }}</span>
        </div>
        <div
          class="chat-message-content chat-message-prose"
          :class="{ 'chat-message-content--dark': isDark }"
          v-html="renderedReplies[node.messageId] || ''"
        />
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import ChatReasoningPeek from './ChatReasoningPeek.vue';
import ChatExecutionCard from './ChatExecutionCard.vue';
import { compactNodeKey, type CompactNode, type ReplyStatus } from '../../features/chat/compactTimeline';
import { renderChatMarkdownForMessage } from '../../features/chat/chatMessageMarkdown';
import type { A2UIUserActionPayload } from '../../features/chat/a2uiUserAction';
import type { ToolUseEvent } from '../../features/chat/types';

const props = defineProps<{
  nodes: CompactNode[];
  isDark: boolean;
  messageId: string;
  isStreaming: boolean;
}>();

const emit = defineEmits<{
  'a2ui-user-action': [payload: A2UIUserActionPayload];
}>();

const { t } = useI18n();

// 计算每个 reply 节点的渲染 HTML（稳定 key 避免流式闪烁）
const renderedReplies = computed<Record<string, string>>(() => {
  const map: Record<string, string> = {};
  for (const node of props.nodes) {
    if (node.kind === 'reply' && !(node.messageId in map)) {
      // 只渲染一次（即使 nodes 重排，也按 messageId 缓存）
      map[node.messageId] = renderChatMarkdownForMessage(
        `${node.messageId}-reply`,
        node.text,
        node.streaming,
      );
    }
  }
  return map;
});

function nodeKey(node: CompactNode): string {
  return compactNodeKey(node);
}

function nodeClasses(node: CompactNode): Record<string, boolean> {
  if (node.kind !== 'reply') return {};
  return {
    'compact-timeline__node--streaming': node.streaming,
    'compact-timeline__node--failed': node.status === 'failed',
    'compact-timeline__node--cancelled': node.status === 'cancelled',
  };
}

function isToolEventCompleted(event: ToolUseEvent): boolean {
  const s = event.status;
  return s === 'success' || s === 'failed' || s === 'cancelled';
}

function replyIcon(status: ReplyStatus): string {
  if (status === 'streaming') return 'progress_activity';
  if (status === 'failed') return 'error_outline';
  if (status === 'cancelled') return 'pause_circle';
  return 'chat_bubble';
}

function replyColor(status: ReplyStatus): string {
  if (status === 'streaming') return 'accent';
  if (status === 'failed') return 'negative';
  if (status === 'cancelled') return 'warning';
  return 'positive';
}
</script>

<style scoped lang="sass">
.compact-timeline
  display: flex
  flex-direction: column
  gap: 4px
  margin: 4px 0

.compact-timeline__node
  display: flex
  flex-direction: column
  gap: 4px
  padding: 2px 0
  border-radius: 8px
  transition: background 0.15s ease

.compact-timeline__node-header
  display: flex
  align-items: center
  gap: 6px
  padding: 2px 4px
  font-size: 12px
  line-height: 1.4

.compact-timeline__node-label
  font-weight: 600
  font-size: 12px
  letter-spacing: 0.02em

.compact-timeline__node-meta
  font-size: 11px
  color: var(--color-text-secondary)
  opacity: 0.85

.compact-timeline__node--streaming
  background: color-mix(in srgb, var(--color-accent) 4%, transparent)
  padding-left: 8px
  margin-left: -8px

.compact-timeline__node--failed
  background: color-mix(in srgb, var(--color-danger) 5%, transparent)
  border-left: 2px solid var(--color-danger)
  padding-left: 6px
  margin-left: -8px

.compact-timeline__node--cancelled
  background: color-mix(in srgb, var(--color-warning) 5%, transparent)
  border-left: 2px solid var(--color-warning)
  padding-left: 6px
  margin-left: -8px
</style>
