<template>
  <div class="timeline-node row no-wrap" :class="{ 'timeline-node--last': isLast }">
    <!-- 左侧时间线轨道 -->
    <div class="timeline-node__track col-auto">
      <div class="timeline-node__dot" :style="{ backgroundColor: dotColor }" />
      <div v-if="!isLast" class="timeline-node__line" />
    </div>

    <!-- 右侧内容区 -->
    <div class="timeline-node__content col">
      <!-- user: 始终展开，不可折叠 -->
      <template v-if="element.kind === 'user'">
        <div class="timeline-node__header row items-center no-wrap q-gutter-xs">
          <q-icon name="person" :style="{ color: dotColor }" size="18px" />
          <span class="text-caption text-weight-medium">{{ t('chat.timeline.user', '用户') }}</span>
        </div>
        <div v-if="element.content" class="timeline-node__body timeline-node__body--user">
          <div class="timeline-node__markdown chat-message-prose" v-html="renderedContent" />
        </div>
      </template>

      <!-- thinking -->
      <template v-else-if="element.kind === 'thinking'">
        <div
          class="timeline-node__header row items-center no-wrap q-gutter-xs"
          :class="{ 'timeline-node__header--clickable': true }"
          @click="emit('toggle')"
        >
          <q-icon name="psychology" :style="{ color: dotColor }" size="18px" />
          <span class="text-caption text-weight-medium">{{ t('chat.timeline.thinking', '思考') }}</span>
          <q-icon v-if="!element.collapsed" name="expand_less" size="14px" style="color: var(--color-text-tertiary)" />
          <q-icon v-else name="expand_more" size="14px" style="color: var(--color-text-tertiary)" />
        </div>
        <div v-if="!element.collapsed && element.reasoning" class="timeline-node__body">
          <pre class="timeline-node__pre">{{ element.reasoning }}</pre>
        </div>
        <div v-else-if="element.reasoning" class="timeline-node__preview text-caption">
          {{ lastLines(element.reasoning, 2) }}
        </div>
      </template>

      <!-- action -->
      <template v-else-if="element.kind === 'action'">
        <div
          class="timeline-node__header row items-center no-wrap q-gutter-xs"
          :class="{ 'timeline-node__header--clickable': true }"
          @click="emit('toggle')"
        >
          <q-icon name="bolt" :style="{ color: dotColor }" size="18px" />
          <span class="text-caption text-weight-medium timeline-node__tool-name">{{ element.toolName || t('chat.timeline.tool', '工具') }}</span>
          <q-icon v-if="toolStatusIcon" :name="toolStatusIcon" :style="{ color: toolStatusColor }" size="14px" />
          <span v-if="element.toolDuration != null" class="text-caption" style="color: var(--color-text-tertiary)">
            {{ formatDuration(element.toolDuration) }}
          </span>
          <q-icon v-if="!element.collapsed" name="expand_less" size="14px" style="color: var(--color-text-tertiary)" />
          <q-icon v-else name="expand_more" size="14px" style="color: var(--color-text-tertiary)" />
        </div>
        <div v-if="!element.collapsed" class="timeline-node__body">
          <div v-if="element.reasoning" class="timeline-node__detail">
            <pre class="timeline-node__pre">{{ element.reasoning }}</pre>
          </div>
          <div v-if="element.toolArguments" class="timeline-node__detail">
            <div class="text-caption text-weight-medium q-mb-xs" style="color: var(--color-text-tertiary)">参数</div>
            <pre class="timeline-node__pre timeline-node__pre--compact">{{ element.toolArguments }}</pre>
          </div>
          <div v-if="element.toolResult" class="timeline-node__detail">
            <div class="text-caption text-weight-medium q-mb-xs" style="color: var(--color-text-tertiary)">结果</div>
            <pre class="timeline-node__pre timeline-node__pre--compact">{{ element.toolResult }}</pre>
          </div>
        </div>
      </template>

      <!-- summary: 始终展开，不可折叠 -->
      <template v-else-if="element.kind === 'summary'">
        <div class="timeline-node__header row items-center no-wrap q-gutter-xs">
          <q-icon name="article" :style="{ color: dotColor }" size="18px" />
          <span class="text-caption text-weight-medium">{{ t('chat.timeline.reply', '回复') }}</span>
        </div>
        <div v-if="element.content" class="timeline-node__body">
          <div class="timeline-node__markdown chat-message-prose" v-html="renderedContent" />
        </div>
      </template>

      <!-- end -->
      <template v-else-if="element.kind === 'end'">
        <div class="timeline-node__header row items-center no-wrap q-gutter-xs">
          <q-icon name="check_circle" :style="{ color: dotColor }" size="18px" />
          <span class="text-caption text-weight-medium">{{ t('chat.timeline.turnComplete', 'Turn 完成') }}</span>
        </div>
      </template>

      <!-- error -->
      <template v-else-if="element.kind === 'error'">
        <div
          class="timeline-node__header row items-center no-wrap q-gutter-xs"
          :class="{ 'timeline-node__header--clickable': true }"
          @click="emit('toggle')"
        >
          <q-icon name="error" :style="{ color: dotColor }" size="18px" />
          <span v-if="element.collapsed" class="text-caption text-weight-medium ellipsis">
            {{ truncate(element.errorMessage, 80) }}
          </span>
          <span v-else class="text-caption text-weight-medium">{{ t('chat.timeline.error', '错误') }}</span>
          <q-icon v-if="!element.collapsed" name="expand_less" size="14px" style="color: var(--color-text-tertiary)" />
          <q-icon v-else name="expand_more" size="14px" style="color: var(--color-text-tertiary)" />
        </div>
        <div v-if="!element.collapsed && element.errorMessage" class="timeline-node__body">
          <pre class="timeline-node__pre timeline-node__pre--error">{{ element.errorMessage }}</pre>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { TimelineElement, TimelineElementKind } from '../../features/chat/timelineTypes';
import { renderChatMarkdown } from '../../features/chat/chatMessageMarkdown';

const props = defineProps<{
  element: TimelineElement;
  isLast: boolean;
}>();

const emit = defineEmits<{
  toggle: [];
}>();

const { t } = useI18n();

const kindColorMap: Record<TimelineElementKind, string> = {
  user: 'var(--color-accent)',
  thinking: 'var(--color-text-secondary)',
  action: 'var(--color-accent)',
  summary: 'var(--color-text-primary)',
  end: 'var(--color-success)',
  error: 'var(--color-danger)',
};

const dotColor = computed(() => kindColorMap[props.element.kind]);

const toolStatusIcon = computed(() => {
  const s = props.element.toolStatus;
  if (s === 'running') return 'hourglass_top';
  if (s === 'success') return 'check_circle';
  if (s === 'failed' || s === 'error') return 'error';
  if (s === 'cancelled') return 'cancel';
  return '';
});

const toolStatusColor = computed(() => {
  const s = props.element.toolStatus;
  if (s === 'running') return 'var(--color-warning)';
  if (s === 'success') return 'var(--color-success)';
  if (s === 'failed' || s === 'error') return 'var(--color-danger)';
  return 'var(--color-text-tertiary)';
});

const renderedContent = computed(() => renderChatMarkdown(props.element.content || ''));

function lastLines(text: string, n: number): string {
  const lines = text.split('\n').filter((l) => l.trim() !== '');
  const tail = lines.slice(-n);
  const joined = tail.join(' ');
  return truncate(joined, 120);
}

function truncate(text: string | undefined, max: number): string {
  if (!text) return '';
  return text.length > max ? text.slice(0, max) + '…' : text;
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const sec = Math.round(ms / 1000);
  if (sec < 60) return `${sec}s`;
  const m = Math.floor(sec / 60);
  const rem = sec % 60;
  return `${m}m ${rem}s`;
}
</script>

<style scoped lang="sass">
.timeline-node
  position: relative

.timeline-node__track
  display: flex
  flex-direction: column
  align-items: center
  width: 20px
  flex-shrink: 0
  padding-top: 4px

.timeline-node__dot
  width: 8px
  height: 8px
  border-radius: 50%
  flex-shrink: 0

.timeline-node__line
  width: 2px
  flex: 1
  background: var(--glass-border)
  margin-top: 4px

.timeline-node__content
  padding-bottom: var(--space-3)
  padding-left: var(--space-2)
  min-width: 0

.timeline-node--last .timeline-node__content
  padding-bottom: 0

.timeline-node__header
  min-height: 24px

.timeline-node__header--clickable
  cursor: pointer
  border-radius: 4px
  padding: 0 var(--space-1)
  margin: calc(-1 * var(--space-1)) calc(-1 * var(--space-1)) 0
  transition: background 0.15s ease

  &:hover
    background: color-mix(in srgb, var(--glass-surface-hover) 60%, transparent)

.timeline-node__body
  margin-top: var(--space-1)
  padding: var(--space-2)
  border-radius: 8px
  background: color-mix(in srgb, var(--glass-surface) 55%, transparent)
  border: 1px solid color-mix(in srgb, var(--glass-border) 40%, transparent)
  animation: timeline-expand 0.2s ease

.timeline-node__body--user
  background: color-mix(in srgb, var(--color-accent) 8%, transparent)
  border-color: color-mix(in srgb, var(--color-accent) 20%, transparent)

.timeline-node__preview
  margin-top: 2px
  padding-left: var(--space-1)
  color: var(--color-text-tertiary)
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis

.timeline-node__pre
  margin: 0
  font-size: var(--text-xs)
  line-height: 1.5
  white-space: pre-wrap
  word-break: break-word
  max-height: 280px
  overflow: auto

.timeline-node__pre--error
  color: var(--color-danger)

.timeline-node__pre--compact
  max-height: 160px
  font-size: 11px
  line-height: 1.4

.timeline-node__detail
  font-size: var(--text-xs)

.timeline-node__tool-name
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
  max-width: 200px

.timeline-node__markdown
  font-size: var(--text-sm)
  line-height: 1.55

@keyframes timeline-expand
  from
    opacity: 0
    max-height: 0
  to
    opacity: 1
    max-height: 500px
</style>
