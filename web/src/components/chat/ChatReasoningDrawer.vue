<template>
  <transition name="chat-reasoning-drawer">
    <aside
      v-if="open && activeReasoning"
      class="chat-reasoning-drawer"
      :class="{ 'chat-reasoning-drawer--dark': isDark }"
      role="complementary"
      :aria-label="t('chat.reasoningTitle', '思考过程')"
    >
      <div class="chat-reasoning-drawer__header row items-center no-wrap">
        <div class="chat-reasoning-drawer__title text-subtitle2 text-weight-medium">
          <q-icon name="psychology" size="18px" class="q-mr-xs" />
          {{ t('chat.reasoningTitle', '思考过程') }}
          <span v-if="activeReasoning.streaming" class="chat-reasoning-drawer__live" aria-hidden="true" />
        </div>
        <q-btn flat dense round icon="close" size="sm" @click="emit('close')" />
      </div>
      <q-separator />
      <div ref="viewportRef" class="chat-reasoning-drawer__body" @wheel="onWheel">
        <!-- eslint-disable vue/no-v-html -- sanitized markdown HTML -->
        <div
          class="chat-reasoning-drawer__content chat-message-prose"
          :class="{ 'chat-message-content--dark': isDark }"
          :style="contentStyle"
          v-html="renderedHtml"
        ></div>
        <!-- eslint-enable vue/no-v-html -->
      </div>
      <div v-if="canScroll && !followTail" class="chat-reasoning-drawer__hint text-caption">
        {{ t('chat.reasoningScrollHint', '滚轮查看更多') }}
      </div>
    </aside>
  </transition>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { renderChatMarkdownForMessage } from '../../features/chat/chatMessageMarkdown';

const props = defineProps<{
  open: boolean;
  activeReasoning: {
    messageId: string;
    reasoning: string;
    streaming: boolean;
  } | null;
  isDark: boolean;
}>();

const emit = defineEmits<{
  close: [];
}>();

const { t } = useI18n();

const viewportRef = ref<HTMLElement | null>(null);
const scrollOffset = ref(0);
const maxScroll = ref(0);
const followTail = ref(true);

const renderedHtml = computed(() => {
  if (!props.activeReasoning) return '';
  return renderChatMarkdownForMessage(
    props.activeReasoning.messageId,
    props.activeReasoning.reasoning,
    props.activeReasoning.streaming,
  );
});

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
  () => props.activeReasoning?.reasoning,
  () => {
    void nextTick(applyTailFollow);
  },
  { immediate: true },
);

watch(
  () => props.activeReasoning?.streaming,
  (live) => {
    if (live) {
      followTail.value = true;
      void nextTick(scrollToTail);
    }
  },
);

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      followTail.value = true;
      void nextTick(scrollToTail);
    }
  },
);

function onWheel(e: WheelEvent) {
  if (maxScroll.value <= 0) return;
  e.preventDefault();
  const step = Math.max(18, Math.abs(e.deltaY));
  const dir = e.deltaY > 0 ? 1 : -1;
  const next = Math.min(maxScroll.value, Math.max(0, scrollOffset.value + dir * step));
  scrollOffset.value = next;
  followTail.value = next >= maxScroll.value - 2;
}
</script>

<style scoped lang="sass">
.chat-reasoning-drawer
  width: 320px
  min-width: 280px
  max-width: 400px
  display: flex
  flex-direction: column
  border-left: 1px solid var(--glass-border)
  background: var(--glass-surface)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))
  border-radius: 0 14px 14px 0
  overflow: hidden

.chat-reasoning-drawer--dark
  background: var(--glass-surface)

.chat-reasoning-drawer__header
  padding: var(--space-2) var(--space-3)
  flex-shrink: 0

.chat-reasoning-drawer__title
  display: flex
  align-items: center
  gap: var(--space-1)
  color: var(--color-text-primary)

.chat-reasoning-drawer__live
  display: inline-block
  width: 6px
  height: 6px
  border-radius: 50%
  background: var(--color-accent)
  animation: chat-reasoning-drawer-pulse 1.5s ease-in-out infinite

.chat-reasoning-drawer__body
  flex: 1
  overflow: hidden
  padding: var(--space-2) var(--space-3)
  position: relative

.chat-reasoning-drawer__content
  will-change: transform

.chat-reasoning-drawer__hint
  padding: var(--space-1) var(--space-3)
  text-align: center
  color: var(--color-text-secondary)
  flex-shrink: 0

.chat-reasoning-drawer-enter-active,
.chat-reasoning-drawer-leave-active
  transition: width 0.22s ease, opacity 0.18s ease, min-width 0.22s ease

.chat-reasoning-drawer-enter-from,
.chat-reasoning-drawer-leave-to
  width: 0
  min-width: 0
  opacity: 0

@keyframes chat-reasoning-drawer-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.3
</style>
