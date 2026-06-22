<template>
  <div class="thinking-area" :class="rootClasses">
    <!-- Streaming (active) state -->
    <template v-if="isActive">
      <div class="thinking-area__icon-wrap">
        <svg
          class="thinking-area__brain"
          viewBox="0 0 24 24"
          fill="none"
          stroke="var(--color-accent-blue)"
          stroke-width="1.5"
        >
          <path d="M12 2C8 2 5 5 5 9c0 2 1 3.5 2 4.5V20a2 2 0 002 2h6a2 2 0 002-2v-6.5c1-1 2-2.5 2-4.5 0-4-3-7-7-7z" />
          <path d="M9 7c1-1 2-1 3 0s2 1 3 0" stroke-opacity="0.5" />
          <path d="M8 11c1-1 2.5-1 4 0s2.5 1 4 0" stroke-opacity="0.3" />
        </svg>
        <span class="thinking-area__flow-light" />
      </div>
      <span class="thinking-area__timer" :class="timerClass">{{ timerText }}</span>
      <span class="thinking-area__content thinking-area__content--active">
        {{ content }}
        <span class="thinking-area__cursor" />
      </span>
    </template>

    <!-- No content (collapsed) state -->
    <template v-else-if="!content">
      <span class="thinking-area__collapsed-btn" @click="expanded = true">
        {{ t('chat.thinking.label') }}
      </span>
    </template>

    <!-- Completed state -->
    <template v-else>
      <!-- Short reasoning: always inline -->
      <template v-if="content.length < 30">
        <div class="thinking-area__icon-wrap thinking-area__icon-wrap--done">
          <svg
            class="thinking-area__brain"
            viewBox="0 0 24 24"
            fill="none"
            stroke="var(--color-accent-blue)"
            stroke-width="1.5"
          >
            <path
              d="M12 2C8 2 5 5 5 9c0 2 1 3.5 2 4.5V20a2 2 0 002 2h6a2 2 0 002-2v-6.5c1-1 2-2.5 2-4.5 0-4-3-7-7-7z"
            />
            <path d="M9 7c1-1 2-1 3 0s2 1 3 0" stroke-opacity="0.5" />
            <path d="M8 11c1-1 2.5-1 4 0s2.5 1 4 0" stroke-opacity="0.3" />
          </svg>
        </div>
        <span class="thinking-area__content thinking-area__content--inline">
          {{ content }}
        </span>
      </template>

      <!-- Collapsed / Expanded with transition -->
      <Transition name="thinking-expand" mode="out-in">
        <!-- Collapsed summary -->
        <div v-if="!expanded" key="collapsed" class="thinking-area__collapsed-wrap" @click="expanded = true">
          <span class="thinking-area__collapsed-btn"> 🧠 {{ summaryText }} </span>
        </div>
        <!-- Expanded full reasoning -->
        <div v-else key="expanded" class="thinking-area__expanded-wrap">
          <div class="thinking-area__icon-wrap thinking-area__icon-wrap--done">
            <svg
              class="thinking-area__brain"
              viewBox="0 0 24 24"
              fill="none"
              stroke="var(--color-accent-blue)"
              stroke-width="1.5"
            >
              <path
                d="M12 2C8 2 5 5 5 9c0 2 1 3.5 2 4.5V20a2 2 0 002 2h6a2 2 0 002-2v-6.5c1-1 2-2.5 2-4.5 0-4-3-7-7-7z"
              />
              <path d="M9 7c1-1 2-1 3 0s2 1 3 0" stroke-opacity="0.5" />
              <path d="M8 11c1-1 2.5-1 4 0s2.5 1 4 0" stroke-opacity="0.3" />
            </svg>
          </div>
          <span class="thinking-area__content thinking-area__content--expanded">
            {{ content }}
          </span>
          <q-btn
            flat
            dense
            round
            icon="unfold_less"
            size="8px"
            class="thinking-area__collapse-btn"
            @click="expanded = false"
          />
        </div>
      </Transition>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue';
import { useI18n } from 'vue-i18n';

const { t } = useI18n();

const props = withDefaults(
  defineProps<{
    content: string;
    isActive?: boolean;
    collapsed?: boolean;
  }>(),
  {
    isActive: true,
    collapsed: false,
  },
);

const expanded = ref(!props.collapsed);

// ── Thinking timer ──
const elapsedMs = ref(0);
let timerInterval: ReturnType<typeof setInterval> | null = null;

watch(
  () => props.isActive,
  (active) => {
    if (active) {
      elapsedMs.value = 0;
      timerInterval = setInterval(() => {
        elapsedMs.value += 1000;
      }, 1000);
    } else {
      if (timerInterval) {
        clearInterval(timerInterval);
        timerInterval = null;
      }
    }
  },
  { immediate: true },
);

onUnmounted(() => {
  if (timerInterval) {
    clearInterval(timerInterval);
    timerInterval = null;
  }
});

const timerText = computed(() => {
  const sec = Math.floor(elapsedMs.value / 1000);
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  const remainSec = sec % 60;
  return `${min}m${remainSec > 0 ? ` ${remainSec}s` : ''}`;
});

const timerClass = computed(() => {
  const sec = elapsedMs.value / 1000;
  if (sec >= 60) return 'thinking-area__timer--danger';
  if (sec >= 30) return 'thinking-area__timer--warning';
  return '';
});

const summaryText = computed(() => {
  const text = props.content || '';
  // Truncate at first period (Chinese or English)
  const periodIdx = text.search(/[。.！!？?]/);
  let firstSentence: string;
  if (periodIdx > 0) {
    firstSentence = text.slice(0, periodIdx + 1);
  } else {
    firstSentence = text;
  }
  return firstSentence.length > 60 ? firstSentence.slice(0, 60) + '…' : firstSentence;
});

const rootClasses = computed(() => ({
  'thinking-area--active': props.isActive,
  'thinking-area--done': !props.isActive && !!props.content,
}));
</script>

<style scoped lang="sass">
.thinking-area
  display: flex
  align-items: flex-start
  gap: 6px
  font-family: var(--text-base)

// ── Brain icon ──
.thinking-area__icon-wrap
  position: relative
  flex-shrink: 0
  width: 18px
  height: 18px
  margin-top: 1px

.thinking-area__brain
  width: 18px
  height: 18px
  display: block

.thinking-area__icon-wrap--done
  .thinking-area__brain
    stroke-opacity: 0.7

// ── Thinking timer ──
.thinking-area__timer
  font-size: var(--text-xs)
  font-weight: 600
  color: var(--color-accent)
  flex-shrink: 0
  font-variant-numeric: tabular-nums
  transition: color 0.3s ease

.thinking-area__timer--warning
  color: var(--color-warning)

.thinking-area__timer--danger
  color: var(--color-danger)

// ── Flow light animation ──
.thinking-area__flow-light
  position: absolute
  inset: 0
  background: linear-gradient(90deg, transparent, color-mix(in srgb, var(--color-primary) 60%, transparent), transparent)
  animation: flowLight 2s ease-in-out infinite
  border-radius: 50%

@keyframes flowLight
  0%
    opacity: 0
    transform: translateX(-8px)
  50%
    opacity: 1
    transform: translateX(0)
  100%
    opacity: 0
    transform: translateX(8px)

// ── Content variants ──
.thinking-area__content
  color: var(--color-text-secondary)
  line-height: 1.5
  transition: max-height 200ms ease, opacity 200ms ease

.thinking-area__content--active
  font-size: var(--text-xs)
  background: color-mix(in srgb, var(--glass-surface) 60%, transparent)
  overflow: hidden
  display: -webkit-box
  -webkit-line-clamp: 2
  -webkit-box-orient: vertical
  border-radius: 4px
  padding: 4px 10px

.thinking-area--active
  border-left: 2px solid color-mix(in srgb, var(--color-primary) 40%, transparent)
  padding-left: 6px
  animation: pulseBorder 2s ease-in-out infinite

@keyframes pulseBorder
  0%, 100%
    border-left-color: color-mix(in srgb, var(--color-primary) 25%, transparent)
  50%
    border-left-color: color-mix(in srgb, var(--color-primary) 55%, transparent)

.thinking-area__content--inline
  font-size: var(--text-xs)
  background: transparent

.thinking-area__content--expanded
  font-size: var(--text-base)
  background: var(--glass-elevated)
  border-radius: 6px
  padding: 8px 10px
  max-height: 300px
  overflow-y: auto

// ── Blinking cursor ──
.thinking-area__cursor
  display: inline-block
  width: 2px
  height: 12px
  background: var(--color-primary)
  animation: blink 0.8s step-end infinite
  vertical-align: middle
  margin-left: 2px

@keyframes blink
  0%, 100%
    opacity: 1
  50%
    opacity: 0

// ── Collapsed button ──
.thinking-area__collapsed-btn
  font-size: var(--text-xs)
  color: var(--color-text-secondary)
  background: color-mix(in srgb, var(--glass-surface) 60%, transparent)
  border-radius: 6px
  padding: 2px 8px
  cursor: pointer
  transition: background 0.15s ease

  &:hover
    background: var(--glass-elevated)

.thinking-area__collapsed-wrap
  cursor: pointer

.thinking-area__expanded-wrap
  display: flex
  align-items: flex-start
  gap: 6px

// ── Expand/collapse transition ──
.thinking-expand-enter-active,
.thinking-expand-leave-active
  transition: max-height 200ms ease, opacity 200ms ease
  overflow: hidden

.thinking-expand-enter-from,
.thinking-expand-leave-to
  max-height: 0
  opacity: 0

// ── Collapse toggle ──
.thinking-area__collapse-btn
  flex-shrink: 0
  margin-top: 2px
  color: var(--color-text-tertiary)
  min-height: unset
</style>
