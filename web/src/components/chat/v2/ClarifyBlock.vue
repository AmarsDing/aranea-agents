<!-- web/src/components/chat/v2/ClarifyBlock.vue
  设计：docs/development/1-chat.design.md §B.10.18.4
  澄清门卡片：分页问答（上一页/下一页/完成），提交后变只读摘要。
  留空 = 按 LLM 推荐执行；推荐项带「推荐」chip 高亮。
-->
<template>
  <div v-if="envelope && questions.length > 0">
    <!-- 交互态：等待用户作答 -->
    <div v-if="step.Status === 'awaiting_input'" class="clarify-block clarify-block--awaiting">
      <div class="clarify-block__header">
        <span class="clarify-block__icon">?</span>
        <span class="clarify-block__label">{{ t('chat.clarify.title') }}</span>
        <span v-if="questions.length > 1" class="clarify-block__page">{{ page + 1 }}/{{ questions.length }}</span>
      </div>

      <div class="clarify-block__question">
        <span class="clarify-block__question-text">{{ currentQuestion.question }}</span>
        <span class="clarify-block__mode-hint">
          {{ currentQuestion.mode === 'multi' ? t('chat.clarify.multiHint') : t('chat.clarify.singleHint') }}
        </span>
      </div>

      <div class="clarify-block__options" :role="currentQuestion.mode === 'multi' ? 'group' : 'radiogroup'">
        <div
          v-for="opt in currentQuestion.options"
          :key="opt"
          class="clarify-block__option"
          :class="{ 'clarify-block__option--selected': isSelected(page, opt) }"
          role="option"
          :aria-selected="isSelected(page, opt)"
          tabindex="0"
          @click="toggleOption(page, opt)"
          @keydown.enter.space.prevent="toggleOption(page, opt)"
        >
          <span
            class="clarify-block__option-box"
            :class="{ 'clarify-block__option-box--radio': currentQuestion.mode !== 'multi' }"
          >
            <span v-if="isSelected(page, opt)" class="clarify-block__option-tick">✓</span>
          </span>
          <span class="clarify-block__option-label">{{ opt }}</span>
          <span v-if="isRecommended(currentQuestion, opt)" class="clarify-block__recommended">
            {{ t('chat.clarify.recommended') }}
          </span>
        </div>
      </div>

      <input
        v-model="others[page]"
        type="text"
        class="clarify-block__other"
        :placeholder="t('chat.clarify.otherPlaceholder')"
        :disabled="submitting"
      />

      <div class="clarify-block__nav">
        <button
          v-if="questions.length > 1"
          class="clarify-block__btn clarify-block__btn--nav"
          :disabled="page === 0 || submitting"
          @click="prevPage"
        >
          {{ t('chat.clarify.prev') }}
        </button>
        <button
          v-if="!isLastPage"
          class="clarify-block__btn clarify-block__btn--nav clarify-block__btn--next"
          :disabled="submitting"
          @click="nextPage"
        >
          {{ t('chat.clarify.next') }}
        </button>
        <button
          v-else
          class="clarify-block__btn clarify-block__btn--finish"
          :disabled="submitting"
          @click="onFinish"
        >
          {{ submitting ? t('chat.clarify.submitting') : t('chat.clarify.finish') }}
        </button>
      </div>
    </div>

    <!-- 只读摘要态：已提交 -->
    <div v-else-if="step.Status === 'completed'" class="clarify-block clarify-block--completed">
      <div class="clarify-block__header">
        <span class="clarify-block__icon clarify-block__icon--done">✓</span>
        <span class="clarify-block__label">{{ t('chat.clarify.submitted') }}</span>
      </div>
      <div v-for="(q, i) in questions" :key="i" class="clarify-block__qa">
        <div class="clarify-block__q">{{ q.question }}</div>
        <div class="clarify-block__a">{{ answerDisplay(i) }}</div>
      </div>
    </div>

    <!-- 取消/失败：内联弱提示 -->
    <div v-else class="clarify-block clarify-block--closed">
      <span class="clarify-block__icon">✗</span>
      <span class="clarify-block__summary">{{ t('chat.clarify.cancelled') }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue';
import { useI18n } from 'vue-i18n';
import type {
  Step,
  ClarificationEnvelope,
  ClarificationQuestion,
  ClarificationAnswer,
} from '../../../features/chat/v2Types';
import type { SubmitClarificationPayload } from '../../../features/chat/types';

// Safe i18n wrapper — falls back to the key when the i18n plugin isn't
// installed (e.g., during unit tests without app.use(i18n)).
function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string) => key };
  }
}

const props = defineProps<{ step: Step }>();
const emit = defineEmits<{
  'submit-clarification': [payload: SubmitClarificationPayload];
}>();

const { t } = useSafeI18n();

// --- Content envelope parsing (fail-open: render nothing on bad JSON) ---
const envelope = computed<ClarificationEnvelope | null>(() => {
  try {
    const parsed = JSON.parse(props.step.Content) as ClarificationEnvelope;
    if (parsed && Array.isArray(parsed.questions)) return parsed;
  } catch {
    /* ignore malformed content */
  }
  return null;
});

const questions = computed<ClarificationQuestion[]>(() => envelope.value?.questions ?? []);
const submittedAnswers = computed<ClarificationAnswer[] | null>(() => envelope.value?.answers ?? null);

// --- Interactive state ---
const page = ref(0);
const selections = ref<string[][]>([]);
const others = ref<string[]>([]);
const submitting = ref(false);
const SUBMIT_TIMEOUT_MS = 15_000;
let submitTimer: ReturnType<typeof setTimeout> | null = null;

// Initialize per-question answer state once questions are known.
watch(
  questions,
  (qs) => {
    if (selections.value.length !== qs.length) {
      selections.value = qs.map(() => []);
      others.value = qs.map(() => '');
      page.value = 0;
    }
  },
  { immediate: true },
);

const currentQuestion = computed<ClarificationQuestion>(() => {
  const qs = questions.value;
  const idx = Math.min(page.value, Math.max(0, qs.length - 1));
  return qs[idx] ?? { question: '', mode: 'single', options: [], recommended: [] };
});

const isLastPage = computed(() => page.value >= questions.value.length - 1);

function isSelected(qIdx: number, opt: string): boolean {
  return selections.value[qIdx]?.includes(opt) ?? false;
}

function isRecommended(q: ClarificationQuestion, opt: string): boolean {
  return Array.isArray(q.recommended) && q.recommended.includes(opt);
}

function toggleOption(qIdx: number, opt: string) {
  if (submitting.value) return;
  const q = questions.value[qIdx];
  if (!q) return;
  const current = selections.value[qIdx] ?? [];
  if (q.mode === 'multi') {
    selections.value[qIdx] = current.includes(opt) ? current.filter((o) => o !== opt) : [...current, opt];
  } else {
    // single：再次点击已选项可取消（恢复「留空 = 按推荐」）
    selections.value[qIdx] = current.includes(opt) ? [] : [opt];
  }
}

function prevPage() {
  if (page.value > 0) page.value -= 1;
}

function nextPage() {
  if (page.value < questions.value.length - 1) page.value += 1;
}

function onFinish() {
  if (submitting.value) return;
  submitting.value = true;
  submitTimer = setTimeout(() => {
    submitting.value = false;
  }, SUBMIT_TIMEOUT_MS);
  emit('submit-clarification', {
    sessionId: props.step.SessionID,
    stepId: props.step.ID,
    answers: questions.value.map((_, i) => ({
      selected: [...(selections.value[i] ?? [])],
      other: (others.value[i] ?? '').trim(),
    })),
  });
}

// Reset submitting state when the step flips (WS step.updated arrives).
watch(
  () => props.step.Status,
  () => {
    submitting.value = false;
    if (submitTimer) {
      clearTimeout(submitTimer);
      submitTimer = null;
    }
  },
);

onUnmounted(() => {
  if (submitTimer) {
    clearTimeout(submitTimer);
    submitTimer = null;
  }
});

// --- Summary rendering ---
function answerDisplay(i: number): string {
  const q = questions.value[i];
  const ans = submittedAnswers.value?.[i];
  if (ans && (ans.selected.length > 0 || (ans.other ?? '').trim() !== '')) {
    const parts = [...ans.selected];
    const other = (ans.other ?? '').trim();
    if (other) parts.push(other);
    return parts.join('、');
  }
  if (q && q.recommended.length > 0) {
    return t('chat.clarify.asRecommended', { value: q.recommended.join('、') });
  }
  return t('chat.clarify.noPreference');
}
</script>

<style lang="sass" scoped>
@keyframes clarify-breathe
  0%, 100%
    border-color: color-mix(in srgb, var(--color-primary) 35%, transparent)
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--color-primary) 20%, transparent)
  50%
    border-color: color-mix(in srgb, var(--color-primary) 65%, transparent)
    box-shadow: 0 0 10px 0 color-mix(in srgb, var(--color-primary) 15%, transparent)

.clarify-block
  border-radius: 10px
  font-size: 13px

  &--awaiting
    padding: 12px 14px
    background: var(--glass-surface)
    border: 1px solid color-mix(in srgb, var(--color-primary) 35%, transparent)
    border-left: 3px solid var(--color-primary)
    animation: clarify-breathe 2.4s ease-in-out infinite

  &--completed
    padding: 10px 12px
    background: var(--glass-surface)
    border: 1px solid var(--glass-border)
    border-left: 3px solid var(--color-success)

  &--closed
    display: flex
    align-items: center
    gap: 6px
    padding: 4px 10px
    border-left: 3px solid var(--color-text-tertiary)
    background: var(--glass-surface)

  &__header
    display: flex
    align-items: center
    gap: 6px
    margin-bottom: 8px

  &__icon
    flex-shrink: 0
    width: 18px
    height: 18px
    border-radius: 50%
    display: flex
    align-items: center
    justify-content: center
    font-size: 12px
    font-weight: 600
    color: var(--color-on-accent, #fff)
    background: var(--color-primary)

    &--done
      background: var(--color-success)

  &__label
    font-weight: 500
    color: var(--color-text-primary)

  &__page
    margin-left: auto
    font-size: 11px
    color: var(--color-text-tertiary)
    font-variant-numeric: tabular-nums

  &__question
    display: flex
    align-items: baseline
    gap: 8px
    margin-bottom: 8px

  &__question-text
    color: var(--color-text-primary)
    line-height: 1.5
    font-weight: 500

  &__mode-hint
    flex-shrink: 0
    font-size: 11px
    color: var(--color-text-tertiary)

  &__options
    display: flex
    flex-direction: column
    gap: 4px
    margin-bottom: 8px

  &__option
    display: flex
    align-items: center
    gap: 8px
    padding: 6px 8px
    border-radius: 8px
    border: 1px solid var(--glass-border)
    cursor: pointer
    transition: border-color 0.15s ease, background 0.15s ease
    user-select: none

    &:hover
      border-color: color-mix(in srgb, var(--color-primary) 45%, transparent)

    &--selected
      border-color: var(--color-primary)
      background: color-mix(in srgb, var(--color-primary) 8%, transparent)

  &__option-box
    flex-shrink: 0
    width: 16px
    height: 16px
    border-radius: 4px
    border: 1.5px solid var(--color-text-tertiary)
    display: flex
    align-items: center
    justify-content: center
    transition: border-color 0.15s ease, background 0.15s ease

    &--radio
      border-radius: 50%

    .clarify-block__option--selected &
      border-color: var(--color-primary)
      background: var(--color-primary)

  &__option-tick
    font-size: 11px
    line-height: 1
    color: var(--color-on-accent, #fff)

  &__option-label
    flex: 1
    color: var(--color-text-primary)
    line-height: 1.4
    word-break: break-word

  &__recommended
    flex-shrink: 0
    font-size: 10px
    padding: 1px 6px
    border-radius: 999px
    color: var(--color-primary)
    border: 1px solid color-mix(in srgb, var(--color-primary) 45%, transparent)
    background: color-mix(in srgb, var(--color-primary) 10%, transparent)

  &__other
    width: 100%
    box-sizing: border-box
    padding: 6px 8px
    margin-bottom: 10px
    border-radius: 8px
    border: 1px solid var(--glass-border)
    background: transparent
    color: var(--color-text-primary)
    font-size: 13px
    outline: none
    transition: border-color 0.15s ease

    &:focus
      border-color: var(--color-primary)

    &::placeholder
      color: var(--color-text-tertiary)

  &__nav
    display: flex
    justify-content: flex-end
    gap: 6px

  &__btn
    padding: 4px 14px
    border-radius: 6px
    border: none
    font-size: 12px
    font-weight: 500
    cursor: pointer
    transition: opacity 0.15s ease
    white-space: nowrap

    &:hover
      opacity: 0.85

    &:disabled
      opacity: 0.4
      cursor: not-allowed

    &--nav
      background: var(--glass-border)
      color: var(--color-text-secondary)

    &--finish
      background: var(--color-primary)
      color: var(--color-on-accent, #fff)

  &__qa
    margin-bottom: 6px

    &:last-child
      margin-bottom: 0

  &__q
    font-size: 12px
    color: var(--color-text-tertiary)
    line-height: 1.4

  &__a
    font-size: 13px
    color: var(--color-text-primary)
    line-height: 1.5

  &__summary
    color: var(--color-text-secondary)
</style>
