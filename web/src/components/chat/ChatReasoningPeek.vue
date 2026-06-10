<template>
  <div
    v-if="thinkingOnly"
    class="chat-reasoning-peek chat-reasoning-peek--thinking-only"
    :class="{ 'chat-reasoning-peek--streaming': streaming }"
    role="status"
    :aria-label="t('chat.reasoningTitle', '思考过程')"
  >
    <span class="chat-reasoning-peek__thinking-text">{{ t('chat.thinking', '正在思考…') }}</span>
    <span v-if="streaming" class="chat-reasoning-peek__cursor" aria-hidden="true" />
  </div>

  <div
    v-else
    class="chat-reasoning-peek"
    :class="{
      'chat-reasoning-peek--expanded': isExpanded,
      'chat-reasoning-peek--streaming': streaming,
      'chat-reasoning-peek--short': isShort,
      'chat-reasoning-peek--dark': isDark,
    }"
    tabindex="0"
    role="region"
    :aria-label="t('chat.reasoningTitle', '思考过程')"
    @click="onClick"
    @wheel="onWheel"
    @keydown.escape="onEscape"
  >
    <!-- Collapsed summary (completed, not short) -->
    <div v-if="!streaming && !isShort && !isExpanded" class="chat-reasoning-peek__summary">
      🧠 {{ summaryText }}
    </div>

    <!-- Streaming or expanded content -->
    <template v-else>
      <div class="chat-reasoning-peek__label">
        {{ t('chat.reasoningTitle', '思考过程') }}
        <span v-if="streaming" class="chat-reasoning-peek__pulse" aria-hidden="true" />
      </div>
      <div ref="viewportRef" class="chat-reasoning-peek__viewport">
        <div
          class="chat-reasoning-peek__content chat-message-prose"
          :class="{ 'chat-message-content--dark': isDark }"
          :style="contentStyle"
          v-html="renderedHtml"
        />
        <span v-if="streaming" class="chat-reasoning-peek__cursor" aria-hidden="true" />
      </div>
      <div v-if="isExpanded && canScroll && !followTail" class="chat-reasoning-peek__hint">
        {{ t('chat.reasoningScrollHint', '滚轮查看更多') }}
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { renderChatMarkdownForMessage } from '../../features/chat/chatMessageMarkdown';

const props = defineProps<{
  messageId: string;
  reasoning: string;
  isDark: boolean;
  streaming?: boolean;
  thinkingOnly?: boolean;
}>();

const { t } = useI18n();

const isExpanded = ref(false);
const scrollOffset = ref(0);
const viewportRef = ref<HTMLElement | null>(null);
const maxScroll = ref(0);
/** Pin viewport to the bottom (last ~2 lines) unless user scrolls up while selected. */
const followTail = ref(true);

// --- Collapse logic ---

/** Strip markdown to plain text for summary extraction. */
const plainText = computed(() => {
  const raw = props.reasoning || '';
  // Quick strip: remove common markdown markers
  return raw
    .replace(/```[\s\S]*?```/g, '')
    .replace(/`[^`]+`/g, '')
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/\*([^*]+)\*/g, '$1')
    .replace(/#{1,6}\s+/g, '')
    .replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
    .replace(/>\s+/g, '')
    .replace(/[-*+]\s+/g, '')
    .trim();
});

/** Short reasoning: < 30 chars, don't collapse. */
const isShort = computed(() => plainText.value.length < 30);

/** Extract first sentence for collapsed summary. */
const summaryText = computed(() => {
  const text = plainText.value;
  if (!text) return '…';
  // Match first sentence boundary: 。.!?！？\n
  const match = text.match(/^(.+?)([。.!?！？\n])/);
  if (match) {
    const first = match[1] + match[2];
    return first.length > 60 ? first.slice(0, 60) + '…' : first;
  }
  return text.length > 60 ? text.slice(0, 60) + '…' : text;
});

// --- Rendering ---

const renderedHtml = computed(() =>
  renderChatMarkdownForMessage(props.messageId, props.reasoning, Boolean(props.streaming)),
);

const canScroll = computed(() => maxScroll.value > 0);

const contentStyle = computed(() =>
  scrollOffset.value > 0 ? { transform: `translateY(-${scrollOffset.value}px)` } : undefined,
);

// --- Scroll management ---

function measureScroll() {
  const vp = viewportRef.value;
  if (!vp) {
    maxScroll.value = 0;
    return;
  }
  const content = vp.firstElementChild as HTMLElement | null;
  if (!content) {
    maxScroll.value = 0;
    return;
  }
  maxScroll.value = Math.max(0, content.scrollHeight - vp.clientHeight);
}

function scrollToTail() {
  measureScroll();
  scrollOffset.value = maxScroll.value;
}

function applyTailFollow() {
  if (followTail.value) {
    scrollToTail();
  } else if (scrollOffset.value > maxScroll.value) {
    scrollOffset.value = maxScroll.value;
  }
}

watch(
  () => props.reasoning,
  () => {
    void nextTick(applyTailFollow);
  },
  { immediate: true },
);

watch(isExpanded, () => {
  void nextTick(applyTailFollow);
});

watch(
  () => props.streaming,
  (live) => {
    if (live) {
      followTail.value = true;
      void nextTick(scrollToTail);
    }
  },
);

// --- Interaction ---

function onClick() {
  // Short reasoning or streaming: no toggle
  if (isShort.value && !props.streaming) return;
  isExpanded.value = !isExpanded.value;
  followTail.value = !isExpanded.value;
  void nextTick(applyTailFollow);
}

function onEscape() {
  isExpanded.value = false;
  followTail.value = true;
  void nextTick(scrollToTail);
}

function onWheel(e: WheelEvent) {
  if (!isExpanded.value || maxScroll.value <= 0) return;
  e.preventDefault();
  const step = Math.max(18, Math.abs(e.deltaY));
  const dir = e.deltaY > 0 ? 1 : -1;
  const next = Math.min(maxScroll.value, Math.max(0, scrollOffset.value + dir * step));
  scrollOffset.value = next;
  followTail.value = next >= maxScroll.value - 2;
}
</script>

<style scoped lang="sass">
// ===== Thinking-only indicator =====
.chat-reasoning-peek--thinking-only
  display: inline-flex
  align-items: center
  gap: 4px
  color: var(--color-text-secondary)
  font-size: var(--q-font-size, 14px)
  font-family: inherit
  padding: 2px 0

.chat-reasoning-peek__thinking-text
  font-style: italic

// ===== Pulse dot (label) =====
.chat-reasoning-peek__pulse
  display: inline-block
  width: 6px
  height: 6px
  border-radius: 50%
  background: var(--color-accent)
  vertical-align: middle
  margin-left: 4px
  animation: peek-pulse 1s ease-in-out infinite

// ===== Blinking cursor =====
.chat-reasoning-peek__cursor
  display: inline-block
  width: 2px
  height: 14px
  background: var(--color-accent)
  vertical-align: middle
  margin-left: 2px
  animation: peek-blink 0.8s step-end infinite

// ===== Collapsed summary line =====
.chat-reasoning-peek__summary
  color: var(--color-text-secondary)
  font-size: var(--q-font-size, 14px)
  font-family: inherit
  line-height: 1.5
  cursor: pointer
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis
  transition: opacity 0.2s ease

  &:hover
    opacity: 0.8

// ===== Keyframes =====
@keyframes peek-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.3

@keyframes peek-blink
  0%, 100%
    opacity: 1
  50%
    opacity: 0

@keyframes pulse-border
  0%, 100%
    border-left-color: var(--color-accent)
  50%
    border-left-color: color-mix(in srgb, var(--color-accent) 30%, transparent)
</style>
