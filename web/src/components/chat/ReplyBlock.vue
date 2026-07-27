<template>
  <div class="reply-block" :class="{ 'reply-block--streaming': streaming }">
    <!-- Card variant -->
    <div class="reply-block__label">
      <span class="reply-block__icon">💬</span>
      <span class="reply-block__label-text">{{ label }}</span>
      <!-- 精灵总结徽章（2026-07-27）：synthesis turn 的 reply = 全部任务总结报告 -->
      <span v-if="isSynthesis" class="reply-block__synthesis-badge">
        {{ t('chat.v2.synthesisBadge') }}
      </span>
      <span v-if="streaming" class="pulse-dot"></span>
    </div>
    <div class="reply-block__content">
      <div v-segmented-markdown="parts" class="reply-block__markdown chat-message-prose"></div>
      <span v-if="streaming" class="cursor-blink"></span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Step } from '../../features/chat/v2Types';
import { renderChatMarkdownParts } from '../../features/chat/chatMessageMarkdown';
import { vSegmentedMarkdown } from '../../features/chat/vSegmentedMarkdown';

// Safe i18n wrapper — falls back to key/fallback when the i18n plugin isn't
// installed (e.g., during unit tests without app.use(i18n)).
function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string, fallback?: string) => fallback ?? key };
  }
}

const props = defineProps<{ step: Step }>();

// --- Bridge computeds: derive legacy fields from Step prop ---
const content = computed(() => props.step.Content);
const streaming = computed(() => props.step.Status === 'running');
const isFinal = computed(() => props.step.IsFinal);
const messageId = computed(() => props.step.ID);

const { t } = useSafeI18n();

// 与后端 biz.SynthesisAuthorAgentKey 对齐：synthesis turn 的 reply step 作者标记。
const SYNTHESIS_AUTHOR_KEY = 'spirit-synthesis';
const isSynthesis = computed(() => props.step.AuthorAgentKey === SYNTHESIS_AUTHOR_KEY);

const label = computed(() =>
  isFinal.value ? t('chat.agentBlock.finalReply') : t('chat.agentBlock.intermediateReply'),
);

const parts = computed(() => renderChatMarkdownParts(messageId.value, content.value, streaming.value));
</script>

<style lang="sass" scoped>
// Agent replies are left-aligned (mirroring user bubbles which are
// right-aligned). Without max-width, the reply block fills the entire
// parent width, making its right edge align with the user bubble's right
// edge — visually indistinguishable from a right-aligned bubble.
// Constraint: 85% keeps long content readable while leaving a visible
// gutter on the right that distinguishes agent from user.
.reply-block
  max-width: 85%
  align-self: flex-start

  &__label
    display: flex
    align-items: center
    gap: 6px
    padding: 4px 0

  &__icon
    font-size: 14px

  &__label-text
    font-size: 13px
    color: var(--color-text-primary)
    font-weight: 600

  // 精灵总结徽章（2026-07-27）：accent 底色胶囊，与普通回复视觉区分
  &__synthesis-badge
    font-size: 11px
    font-weight: 600
    line-height: 1
    padding: 3px 8px
    border-radius: 999px
    color: var(--color-accent)
    background: color-mix(in srgb, var(--color-accent) 14%, transparent)
    border: 1px solid color-mix(in srgb, var(--color-accent) 35%, transparent)

  &__content
    padding: 10px 14px
    background: var(--glass-elevated)
    border: 1px solid var(--glass-border)
    border-radius: 12px
    font-size: 14px
    line-height: 1.7
    word-break: break-word

// 夜间助手气泡切换为标准玻璃 token（§6.14 要求夜 --glass-surface）
body.body--dark .reply-block__content
  background: var(--glass-surface)

// Chat UI fix: streaming state — pulsing left border to signal "agent is
// still typing". Pairs with the existing pulse-dot + cursor-blink inside.
.reply-block--streaming
  .reply-block__content
    border-left: 2px solid var(--color-accent)
    box-shadow: -1px 0 0 var(--color-accent)
    animation: reply-streaming-pulse 1.6s ease-in-out infinite

@keyframes reply-streaming-pulse
  0%, 100%
    border-left-color: var(--color-accent)
  50%
    border-left-color: color-mix(in srgb, var(--color-accent) 40%, transparent)
</style>
