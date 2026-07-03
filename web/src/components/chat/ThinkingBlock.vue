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
        'thinking-block--inline-short': inlineNoCollapse,
        'thinking-block--dark': isDark,
      },
    ]"
    tabindex="0"
    role="region"
    :aria-label="t('chat.reasoningTitle', '思考过程')"
    @click="onClick"
    @keydown.escape="onEscape"
  >
    <!-- ===== Streaming + collapsed: status indicator (existing behavior) ===== -->
    <div
      v-if="streaming && collapsed"
      class="thinking-block__streaming-indicator thinking-block__streaming-indicator--card"
      @click.stop="onClick"
    >
      <q-icon name="psychology_alt" size="14px" color="accent" />
      <span class="thinking-block__streaming-text">{{ displayLabel }}</span>
      <span class="thinking-block__pulse" aria-hidden="true" />
      <span v-if="durationMs != null" class="thinking-block__duration">{{ formattedDuration }}</span>
      <span class="thinking-block__toggle">{{ collapsed ? '▶' : '▼' }}</span>
    </div>

    <!-- ===== US-24/§6.8.3: Completed inline-no-collapse (reasoning < 30 chars) ===== -->
    <span v-else-if="inlineNoCollapse" class="thinking-block__inline-short" :title="t('chat.reasoningTitle')">
      <span class="thinking-block__icon-emoji" aria-hidden="true">🧠</span>
      <span class="thinking-block__inline-text">{{ reasoning }}</span>
    </span>

    <!-- ===== US-24/§6.8.3: Completed collapsed (single-line inline span) ===== -->
    <span
      v-else-if="collapsed"
      class="thinking-block__collapsed"
      :class="{ 'thinking-block__collapsed--dark': isDark }"
      :title="collapsedHint"
      role="region"
      :aria-label="t('chat.reasoningTitle')"
    >
      <span class="thinking-block__icon-emoji" aria-hidden="true">🧠</span>
      <span class="thinking-block__collapsed-text">{{ summary }}</span>
      <span class="thinking-block__collapsed-toggle" aria-hidden="true">▶</span>
    </span>

    <!-- ===== Expanded (streaming live or user-clicked): label row + body ===== -->
    <template v-else>
      <div class="thinking-block__label">
        <span class="thinking-block__icon-emoji" aria-hidden="true">🧠</span>
        <span class="thinking-block__label-text">{{ displayLabel }}</span>
        <span v-if="streaming" class="thinking-block__pulse" aria-hidden="true" />
        <span v-if="durationMs != null" class="thinking-block__duration">{{ formattedDuration }}</span>
        <span class="thinking-block__toggle" @click.stop="onClick">▼</span>
      </div>
      <div class="thinking-block__collapse-wrapper">
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
import type { Step } from '../../features/chat/v2Types';
import { renderChatMarkdownForMessage } from '../../features/chat/chatMessageMarkdown';
import { formatDuration } from '../../features/chat/agentTreeUtils';
import { useCollapseState } from '../../features/chat/composables/useCollapseState';

// Safe i18n wrapper — falls back to key/fallback when the i18n plugin isn't
// installed (e.g., during unit tests without app.use(i18n)).
function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string, fallback?: string) => fallback ?? key };
  }
}

const props = withDefaults(
  defineProps<{
    /** v2 Step entity — replaces legacy messageId/reasoning/streaming/... props */
    step: Step;
    /** 深色模式 */
    isDark?: boolean;
    /** 默认折叠状态：true=收起，false=展开。流式和结束后均默认折叠 */
    defaultCollapsed?: boolean;
    /** 思考块标签（"规划"/"推理"/"重规划"/"进度"等）。
     * 后端 Activity.label 透传；为空时回退到 i18n 默认"思考"。 */
    label?: string;
  }>(),
  {
    isDark: false,
    defaultCollapsed: true,
    label: '',
  },
);

const { t } = useSafeI18n();

// --- Bridge computeds: derive legacy fields from Step prop ---
const messageId = computed(() => props.step.ID);
const reasoning = computed(() => props.step.Reasoning);
const streaming = computed(() => props.step.Status === 'running');
const thinkingOnly = computed(() => !props.step.Reasoning && props.step.Status === 'running');
const durationMs = computed(() => props.step.ToolDurationMs || null);

/**
 * Resolves the display label: prefer the explicit `label` prop (set by
 * backend via Activity.label to distinguish "规划"/"推理"/"重规划"/"进度"),
 * fall back to the i18n-default "思考" key when not provided.
 *
 * Chat UI fix: previously the component hardcoded `t('chat.thinking.summary', '思考')`
 * everywhere, ignoring the label field that useActivityTimeline already
 * passes through from backend Activity data.
 */
const displayLabel = computed(() => {
  const trimmed = typeof props.label === 'string' ? props.label.trim() : '';
  return trimmed || t('chat.thinking.summary', '思考');
});

// --- Collapse state (T8.4: persisted to sessionStorage) ---

const { collapsed, toggle, setCollapsed } = useCollapseState(`thinking:${messageId.value}`, props.defaultCollapsed);
// Sync with external defaultCollapsed changes (e.g., when Activity data updates from AF).
// Only apply when not streaming — streaming state is managed by the streaming watch.
// Note: only applies if the user hasn't explicitly toggled (sessionStorage has no stored value).
watch(
  () => props.defaultCollapsed,
  (val) => {
    if (!streaming.value) setCollapsed(val);
  },
);
const viewportRef = ref<HTMLElement | null>(null);

/** Whether user has scrolled up away from the bottom. */
const userScrolledUp = ref(false);

/**
 * P2-B: Tracks whether the user has manually toggled the collapse state
 * during the current streaming session. When the user expands/collapses
 * manually, streaming-end must NOT force-collapse (which would override
 * their intent). Only auto-collapse on streaming-end if the user never
 * toggled — preserving the "collapse to keep timeline compact" default.
 */
const userToggled = ref(false);

// --- Plain text extraction ---

/**
 * US-24/§6.8.3: Summary text for collapsed state.
 * First sentence (split by 。.!?！？\n), truncated at 60 chars + ….
 * Used by single-line inline collapsed span.
 */
const summary = computed(() => {
  const text = reasoning.value || '';
  const firstSentence = text.split(/[。.!?！？\n]/)[0] || '';
  return firstSentence.length > 60 ? firstSentence.slice(0, 60) + '…' : firstSentence;
});

/**
 * US-24/§6.8.3 exception: reasoning < 30 chars renders inline without
 * collapse toggle (information density too low, collapse would distract).
 * Only applies to completed (non-streaming) state.
 */
const inlineNoCollapse = computed(() => !streaming.value && (reasoning.value || '').length < 30);

/** Hover hint for collapsed state: includes label + duration if present. */
const collapsedHint = computed(() => {
  const parts: string[] = [];
  if (props.label) parts.push(props.label);
  if (durationMs.value != null) parts.push(formatDuration(durationMs.value));
  parts.push(t('chat.thinking.clickToExpand'));
  return parts.join(' · ');
});

const formattedDuration = computed(() => (durationMs.value != null ? formatDuration(durationMs.value) : ''));

// --- Rendering ---

const renderedHtml = computed(() => {
  return renderChatMarkdownForMessage(messageId.value, reasoning.value, Boolean(streaming.value));
});

// --- Native scroll management ---

function scrollToBottom() {
  const vp = viewportRef.value;
  if (vp) {
    vp.scrollTop = vp.scrollHeight;
  }
}

// Auto-scroll during streaming
watch(reasoning, () => {
  if (streaming.value && !userScrolledUp.value) {
    void nextTick(scrollToBottom);
  }
});

// When streaming starts, expand so users can follow the reasoning live.
// When streaming ends, only auto-collapse if the user hasn't manually toggled
// during this streaming session — respecting their expand/collapse intent
// (P2-B: the previous behavior force-collapsed on streaming-end, overriding
// a user who expanded to read along).
watch(streaming, (live) => {
  if (live) {
    userScrolledUp.value = false;
    userToggled.value = false;
    setCollapsed(false);
    void nextTick(scrollToBottom);
  } else if (!userToggled.value) {
    // US-24/§6.8.3: < 30 chars → inline no-collapse (don't auto-collapse)
    if (inlineNoCollapse.value) {
      setCollapsed(false);
    } else {
      // streaming 结束时折叠（仅当用户未手动操作时）
      setCollapsed(props.defaultCollapsed);
    }
  }
});

// US-24/§6.8.3: when reasoning shrinks below 30 chars (e.g., trimmed) or
// streaming ends with short content, force-expand to trigger inline render.
watch(inlineNoCollapse, (val) => {
  if (val && !userToggled.value) setCollapsed(false);
});

// --- Interaction ---

function onClick() {
  // US-24/§6.8.3: inline-short (< 30 chars) is non-interactive — no toggle.
  if (inlineNoCollapse.value) return;
  userToggled.value = true;
  toggle();

  // After expanding, scroll to bottom if streaming
  if (!collapsed.value && streaming.value) {
    userScrolledUp.value = false;
    void nextTick(scrollToBottom);
  }
}

function onEscape() {
  userToggled.value = true;
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

// US-24/§6.8.3: collapsed and inline-short states render inline (single-line span)
.thinking-block--card.thinking-block--collapsed,
.thinking-block--card.thinking-block--inline-short
  display: inline
  margin-bottom: 0

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
  cursor: pointer

// ===== US-24/§6.8.3: Inline collapsed span (🧠 + summary + ▶) =====
.thinking-block__icon-emoji
  font-size: 14px
  line-height: inherit
  vertical-align: -1px
  margin-right: 4px

.thinking-block__collapsed
  display: inline
  cursor: pointer
  color: var(--color-text-secondary)
  font-size: var(--q-font-size, 14px)
  line-height: var(--q-line-height, 1.5)
  background: transparent
  padding: 2px 0
  user-select: none
  transition: color 0.15s ease

  &:hover
    color: var(--color-text-primary)

.thinking-block__collapsed-text
  color: var(--color-text-tertiary)

.thinking-block__collapsed-toggle
  margin-left: 4px
  color: var(--color-text-tertiary)
  font-size: 10px

// ===== US-24/§6.8.3: Inline short (reasoning < 30 chars, no toggle) =====
.thinking-block__inline-short
  display: inline
  color: var(--color-text-secondary)
  font-size: var(--q-font-size, 14px)
  line-height: var(--q-line-height, 1.5)
  background: transparent
  padding: 2px 0

  .thinking-block__inline-text
    color: var(--color-text-tertiary)

.thinking-block--card .thinking-block__body
  padding: 8px 12px
  margin-left: 20px
  background: transparent  // US-24/§6.8.3: expanded completed state has transparent bg
  border-left: 2px solid var(--glass-border)
  border-radius: 0 8px 8px 0
  font-size: $font-size-card
  color: var(--color-text-secondary)
  line-height: 1.6
  max-height: 200px
  overflow-y: auto
  transition: border-color 0.3s ease

.thinking-block--card .thinking-block__body--streaming
  background: $bg-subtle  // streaming keeps subtle bg per §6.8.1
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
