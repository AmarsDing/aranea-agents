<template>
  <div
    class="chat-reasoning-peek"
    :class="{
      'chat-reasoning-peek--expanded': expanded,
      'chat-reasoning-peek--streaming': streaming,
      'chat-reasoning-peek--dark': isDark,
    }"
    tabindex="0"
    role="region"
    :aria-label="t('chat.reasoningTitle', '思考过程')"
    @click="onClick"
    @wheel="onWheel"
    @keydown.escape="clearSelection"
  >
    <div class="chat-reasoning-peek__label text-caption text-weight-medium">
      {{ t('chat.reasoningTitle', '思考过程') }}
      <span v-if="streaming" class="chat-reasoning-peek__pulse" aria-hidden="true" />
    </div>
    <div v-if="thinkingOnly" class="chat-reasoning-peek__thinking chat-thinking-pulse text-caption text-grey-7">
      {{ t('chat.thinking', '正在思考…') }}
    </div>
    <div v-else ref="viewportRef" class="chat-reasoning-peek__viewport">
      <div
        class="chat-reasoning-peek__content chat-message-prose"
        :class="{ 'chat-message-content--dark': isDark }"
        :style="contentStyle"
        v-html="renderedHtml"
      />
      <span v-if="streaming" class="chat-reasoning-peek__cursor" aria-hidden="true" />
    </div>
    <div v-if="expanded && canScroll && !followTail" class="chat-reasoning-peek__hint text-caption">
      {{ t('chat.reasoningScrollHint', '滚轮查看更多') }}
    </div>
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

const expanded = ref(false);
const scrollOffset = ref(0);
const viewportRef = ref<HTMLElement | null>(null);
const maxScroll = ref(0);
/** Pin viewport to the bottom (last ~2 lines) unless user scrolls up while selected. */
const followTail = ref(true);

const renderedHtml = computed(() =>
  renderChatMarkdownForMessage(props.messageId, props.reasoning, Boolean(props.streaming)),
);

const canScroll = computed(() => maxScroll.value > 0);

const contentStyle = computed(() =>
  scrollOffset.value > 0 ? { transform: `translateY(-${scrollOffset.value}px)` } : undefined,
);

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

watch(expanded, () => {
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

function onClick() {
  expanded.value = !expanded.value;
  followTail.value = !expanded.value;
  void nextTick(applyTailFollow);
}

function clearSelection() {
  expanded.value = false;
  followTail.value = true;
  void nextTick(scrollToTail);
}

function onWheel(e: WheelEvent) {
  if (!expanded.value || maxScroll.value <= 0) return;
  e.preventDefault();
  const step = Math.max(18, Math.abs(e.deltaY));
  const dir = e.deltaY > 0 ? 1 : -1;
  const next = Math.min(maxScroll.value, Math.max(0, scrollOffset.value + dir * step));
  scrollOffset.value = next;
  followTail.value = next >= maxScroll.value - 2;
}
</script>

<style scoped lang="sass">
.chat-reasoning-peek__pulse
  display: inline-block
  width: 6px
  height: 6px
  border-radius: 50%
  background: var(--color-accent)
  vertical-align: middle
  margin-left: 4px
  animation: peek-pulse 1s ease-in-out infinite

.chat-reasoning-peek__cursor
  display: inline-block
  width: 2px
  height: 14px
  background: var(--color-accent)
  vertical-align: middle
  margin-left: 2px
  animation: peek-blink 0.8s step-end infinite

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
</style>
