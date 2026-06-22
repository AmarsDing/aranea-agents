<template>
  <!-- thinkingOnly mode: just show "thinking..." indicator -->
  <div
    v-if="thinkingOnly"
    class="thinking-block thinking-block--thinking-only"
    :class="{ 'thinking-block--streaming': streaming }"
    role="status"
    :aria-label="t('chat.reasoningTitle', '思考过程')"
  >
    <span class="thinking-block__thinking-text">{{ t('chat.thinking.thinking', '正在思考…') }}</span>
    <span class="thinking-block__pulse" aria-hidden="true" />
    <span v-if="streaming" class="thinking-block__cursor" aria-hidden="true" />
  </div>

  <!-- Main component -->
  <div
    v-else
    class="thinking-block thinking-block--card"
    :class="[
      {
        'thinking-block--streaming': streaming,
        'thinking-block--collapsed': collapsed,
        'thinking-block--dark': isDark,
      },
    ]"
    tabindex="0"
    role="region"
    :aria-label="t('chat.reasoningTitle', '思考过程')"
    @click="onClick"
    @keydown.escape="onEscape"
  >
    <!-- ===== card variant ===== -->
    <!-- Streaming + collapsed: status indicator only -->
    <div
      v-if="streaming && collapsed"
      class="thinking-block__streaming-indicator thinking-block__streaming-indicator--card"
      @click.stop="onClick"
    >
      <q-icon name="psychology_alt" size="14px" color="accent" />
      <span class="thinking-block__streaming-text">{{ t('chat.thinking.summary', '思考') }}</span>
      <span class="thinking-block__pulse" aria-hidden="true" />
      <span v-if="durationMs != null" class="thinking-block__duration">{{ formattedDuration }}</span>
      <span class="thinking-block__toggle">{{ collapsed ? '▶' : '▼' }}</span>
    </div>

    <!-- Label row (not streaming or expanded) -->
    <template v-else>
      <div class="thinking-block__label">
        <q-icon name="psychology_alt" size="14px" color="accent" class="thinking-block__label-icon" />
        <span class="thinking-block__label-text">{{ t('chat.thinking.summary', '思考') }}</span>
        <span v-if="streaming" class="thinking-block__pulse" aria-hidden="true" />
        <span v-if="durationMs != null" class="thinking-block__duration">{{ formattedDuration }}</span>
        <span class="thinking-block__toggle">{{ collapsed ? '▶' : '▼' }}</span>
      </div>

      <!-- Collapsed: preview text -->
      <div v-if="collapsed" class="thinking-block__preview">{{ previewText }}</div>

      <!-- Expanded content -->
      <div v-else class="thinking-block__collapse-wrapper">
        <div class="thinking-block__collapse-inner">
          <div class="thinking-block__body" :class="{ 'thinking-block__body--streaming': streaming }">
            <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->
            <div class="thinking-block__content chat-message-prose" v-html="renderedHtml" />
            <span v-if="streaming" class="thinking-block__cursor" aria-hidden="true" />
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { renderChatMarkdownForMessage } from '../../features/chat/chatMessageMarkdown';
import { formatDuration } from '../../features/chat/agentTreeUtils';
import { useCollapseState } from '../../features/chat/composables/useCollapseState';

const props = withDefaults(
  defineProps<{
    /** 消息 ID（用于 Markdown 渲染缓存 key） */
    messageId: string;
    /** 思考内容（Markdown 文本） */
    reasoning: string;
    /** 是否正在流式输出 */
    streaming?: boolean;
    /** 是否仅显示"正在思考"指示器（无内容时） */
    thinkingOnly?: boolean;
    /** 思考耗时（毫秒），null 表示未知 */
    durationMs?: number | null;
    /** 深色模式 */
    isDark?: boolean;
    /** 默认折叠状态：true=收起，false=展开。流式和结束后均默认折叠 */
    defaultCollapsed?: boolean;
  }>(),
  {
    streaming: false,
    thinkingOnly: false,
    durationMs: null,
    isDark: false,
    defaultCollapsed: true,
  },
);

const { t } = useI18n();

// --- Collapse state (T8.4: persisted to sessionStorage) ---

const { collapsed, toggle, setCollapsed } = useCollapseState(`thinking:${props.messageId}`, props.defaultCollapsed);
// Sync with external defaultCollapsed changes (e.g., when Activity data updates from AF).
// Only apply when not streaming — streaming state is managed by the streaming watch.
// Note: only applies if the user hasn't explicitly toggled (sessionStorage has no stored value).
watch(
  () => props.defaultCollapsed,
  (val) => {
    if (!props.streaming) setCollapsed(val);
  },
);
const viewportRef = ref<HTMLElement | null>(null);

/** Whether user has scrolled up away from the bottom. */
const userScrolledUp = ref(false);

// --- Plain text extraction ---

/** Preview text for card collapsed state. */
const previewText = computed(() => {
  const content = props.reasoning || '';
  const firstLine = content.split('\n').find((l) => l.trim() !== '') || '';
  return firstLine.length > 80 ? firstLine.slice(0, 80) + '…' : firstLine;
});

const formattedDuration = computed(() => (props.durationMs != null ? formatDuration(props.durationMs) : ''));

// --- Rendering ---

const renderedHtml = computed(() => {
  return renderChatMarkdownForMessage(props.messageId, props.reasoning, Boolean(props.streaming));
});

// --- Native scroll management ---

function scrollToBottom() {
  const vp = viewportRef.value;
  if (vp) {
    vp.scrollTop = vp.scrollHeight;
  }
}

// Auto-scroll during streaming
watch(
  () => props.reasoning,
  () => {
    if (props.streaming && !userScrolledUp.value) {
      void nextTick(scrollToBottom);
    }
  },
);

// When streaming starts, reset scroll follow but stay collapsed
watch(
  () => props.streaming,
  (live) => {
    if (live) {
      userScrolledUp.value = false;
      setCollapsed(true); // 流式时保持折叠，显示状态指示器
      void nextTick(scrollToBottom);
    } else {
      // streaming 结束时保持折叠
      setCollapsed(true);
    }
  },
);

// --- Interaction ---

function onClick() {
  toggle();

  // After expanding, scroll to bottom if streaming
  if (!collapsed.value && props.streaming) {
    userScrolledUp.value = false;
    void nextTick(scrollToBottom);
  }
}

function onEscape() {
  setCollapsed(true);
}
</script>

<style scoped lang="sass">
// ===== Shared variables =====
$font-size-inline: var(--q-font-size, 14px)
$font-size-card: 13px
$bg-subtle: color-mix(in srgb, var(--color-text-primary) 3%, transparent)
$border-accent: color-mix(in srgb, var(--color-accent) 40%, transparent)

// ===== thinkingOnly indicator =====
.thinking-block--thinking-only
  display: inline-flex
  align-items: center
  gap: 4px
  color: var(--color-text-secondary)
  font-size: $font-size-inline
  font-family: inherit
  padding: 2px 0

.thinking-block__thinking-text
  font-style: italic

// ===== Streaming indicator (collapsed + streaming) =====
.thinking-block__streaming-indicator
  display: flex
  align-items: center
  gap: 6px
  cursor: pointer
  color: var(--color-text-secondary)
  font-size: $font-size-inline
  font-family: inherit
  padding: 2px 0
  transition: opacity 0.15s ease

  &:hover
    opacity: 0.8

.thinking-block__streaming-indicator--card
  padding: 4px 0
  font-size: $font-size-card

  &:hover
    background: color-mix(in srgb, var(--color-text-primary) 4%, transparent)
    border-radius: 4px

.thinking-block__streaming-text
  font-style: italic

// ===== Pulse dot =====
.thinking-block__pulse
  display: inline-block
  width: 6px
  height: 6px
  border-radius: 50%
  background: var(--color-accent)
  vertical-align: middle
  margin-left: 4px
  animation: thinking-pulse 1s ease-in-out infinite

// ===== Blinking cursor =====
.thinking-block__cursor
  display: inline-block
  width: 2px
  height: 14px
  background: var(--color-accent)
  vertical-align: middle
  margin-left: 2px
  animation: thinking-blink 0.8s step-end infinite

// ===== Duration badge =====
.thinking-block__duration
  color: var(--color-text-tertiary)
  font-size: 11px
  margin-left: 4px

// ===== Collapse/expand animation (grid-template-rows) =====
.thinking-block__collapse-wrapper
  display: grid
  grid-template-rows: 1fr
  transition: grid-template-rows 0.25s ease

.thinking-block--collapsed .thinking-block__collapse-wrapper
  grid-template-rows: 0fr

.thinking-block__collapse-inner
  overflow: hidden

// ===== card variant =====
.thinking-block--card
  margin-bottom: 4px

.thinking-block--card .thinking-block__label
  display: flex
  align-items: center
  gap: 6px
  cursor: pointer
  padding: 4px 0
  user-select: none
  transition: background 0.12s

  &:hover
    background: color-mix(in srgb, var(--color-text-primary) 4%, transparent)
    border-radius: 4px

.thinking-block__label-icon
  flex-shrink: 0

.thinking-block__label-text
  font-size: $font-size-card
  color: var(--color-text-secondary)
  font-weight: 500

.thinking-block__toggle
  margin-left: auto
  color: var(--color-text-tertiary)
  font-size: 10px

.thinking-block--card .thinking-block__preview
  font-size: $font-size-card
  color: var(--color-text-tertiary)
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
  max-width: 100%
  padding-left: 20px

.thinking-block--card .thinking-block__body
  padding: 8px 12px
  margin-left: 20px
  background: $bg-subtle
  border-left: 2px solid var(--glass-border)
  border-radius: 0 8px 8px 0
  font-size: $font-size-card
  color: var(--color-text-secondary)
  line-height: 1.6
  max-height: 200px
  overflow-y: auto
  transition: border-color 0.3s ease

.thinking-block--card .thinking-block__body--streaming
  border-left-color: $border-accent

// ===== Keyframes =====
@keyframes thinking-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.3

@keyframes thinking-blink
  0%, 100%
    opacity: 1
  50%
    opacity: 0
</style>
