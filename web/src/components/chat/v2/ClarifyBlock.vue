<!-- web/src/components/chat/v2/ClarifyBlock.vue
  设计：docs/development/1-chat.design.md §B.10.18.4
  澄清门卡片：标题「提问」带图标，标题栏按状态配色（等待=accent 淡底+呼吸 / 完成=成功绿淡底）。
  等待态可人工折叠；分页问答（上一页/下一页/完成），提交后自动折叠为只读摘要，展开可核对已记录的作答。
  留空 = 按 LLM 推荐执行；推荐项带「推荐」chip 高亮。
-->
<template>
  <div v-if="envelope && questions.length > 0">
    <!-- 交互态：等待用户作答（标题栏可点击人工折叠） -->
    <div v-if="step.Status === 'awaiting_input'" class="clarify-block clarify-block--awaiting">
      <button
        type="button"
        class="clarify-block__header"
        :aria-expanded="!collapsed"
        @click="collapsed = !collapsed"
      >
        <span class="clarify-block__icon">
          <q-icon name="help_outline" size="13px" />
        </span>
        <span class="clarify-block__label">{{ t('chat.clarify.title') }}</span>
        <span v-if="collapsed" class="clarify-block__pending">{{ t('chat.clarify.pending') }}</span>
        <span v-else-if="questions.length > 1" class="clarify-block__page">{{ page + 1 }}/{{ questions.length }}</span>
        <q-icon
          name="expand_more"
          size="18px"
          class="clarify-block__chevron"
          :class="{ 'clarify-block__chevron--expanded': !collapsed }"
          aria-hidden="true"
        />
      </button>

      <div v-show="!collapsed" class="clarify-block__body">
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
    </div>

    <!-- 只读摘要态：已提交（自动折叠，点击标题栏展开核对已记录的作答） -->
    <div v-else-if="step.Status === 'completed'" class="clarify-block clarify-block--completed">
      <button
        type="button"
        class="clarify-block__header"
        :aria-expanded="summaryExpanded"
        @click="summaryExpanded = !summaryExpanded"
      >
        <span class="clarify-block__icon clarify-block__icon--done">
          <q-icon name="check" size="13px" />
        </span>
        <span class="clarify-block__label">{{ t('chat.clarify.title') }}</span>
        <q-icon
          name="expand_more"
          size="18px"
          class="clarify-block__chevron"
          :class="{ 'clarify-block__chevron--expanded': summaryExpanded }"
          aria-hidden="true"
        />
      </button>
      <div v-show="summaryExpanded" class="clarify-block__qa-list">
        <div v-for="(q, i) in questions" :key="i" class="clarify-block__qa">
          <div class="clarify-block__q">{{ q.question }}</div>
          <div class="clarify-block__a">{{ answerDisplay(i) }}</div>
        </div>
      </div>
    </div>

    <!-- 取消/失败：内联弱提示 -->
    <div v-else class="clarify-block clarify-block--closed">
      <span class="clarify-block__closed-icon">✗</span>
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
// 等待态：人工折叠（默认展开待作答）
const collapsed = ref(false);
// 完成态摘要：自动折叠，点击标题栏展开查看问答记录
const summaryExpanded = ref(false);
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
  // 跳过的问题后端回写 selected=null（Go nil slice），必须空值安全。
  const selected = ans?.selected ?? [];
  const other = (ans?.other ?? '').trim();
  if (selected.length > 0 || other !== '') {
    const parts = [...selected];
    if (other) parts.push(other);
    return parts.join('、');
  }
  const recommended = q?.recommended ?? [];
  if (recommended.length > 0) {
    return t('chat.clarify.asRecommended', { value: recommended.join('、') });
  }
  return t('chat.clarify.noPreference');
}
</script>

<style lang="sass" scoped>
@keyframes clarify-breathe
  0%, 100%
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--color-accent) 16%, transparent)
  50%
    box-shadow: 0 0 12px 0 color-mix(in srgb, var(--color-accent) 10%, transparent)

.clarify-block
  border-radius: 12px
  overflow: hidden
  font-size: 13px
  background: var(--glass-surface)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))

  &--awaiting
    border: 1px solid color-mix(in srgb, var(--color-accent) 35%, transparent)
    animation: clarify-breathe 2.4s ease-in-out infinite

  &--completed
    border: 1px solid var(--glass-border)

  &--closed
    display: flex
    align-items: center
    gap: 6px
    padding: 6px 12px
    border: 1px solid var(--glass-border)

  // ---- 标题栏（状态配色：等待=accent 淡底 / 完成=成功绿淡底） ----
  &__header
    display: flex
    align-items: center
    gap: 8px
    width: 100%
    margin: 0
    padding: 8px 12px
    border: none
    cursor: pointer
    font: inherit
    text-align: left
    transition: background 0.15s ease

    &:focus-visible
      outline: 2px solid color-mix(in srgb, var(--color-accent) 45%, transparent)
      outline-offset: -2px

    .clarify-block--awaiting > &
      background: color-mix(in srgb, var(--color-accent) 10%, transparent)

      &:hover
        background: color-mix(in srgb, var(--color-accent) 16%, transparent)

    .clarify-block--completed > &
      background: color-mix(in srgb, var(--color-success) 10%, transparent)

      &:hover
        background: color-mix(in srgb, var(--color-success) 16%, transparent)

  &__icon
    flex-shrink: 0
    width: 20px
    height: 20px
    border-radius: 50%
    display: flex
    align-items: center
    justify-content: center
    color: var(--color-on-accent)
    background: var(--color-accent)

    &--done
      background: var(--color-success)

  &__label
    font-weight: 600
    color: var(--color-text-heading)

  &__pending
    margin-left: auto
    flex-shrink: 0
    font-size: 11px
    padding: 1px 8px
    border-radius: 999px
    color: var(--color-text-tertiary)
    border: 1px solid var(--glass-border)

  &__page
    margin-left: auto
    flex-shrink: 0
    font-size: 11px
    color: var(--color-text-tertiary)
    font-variant-numeric: tabular-nums

  &__chevron
    flex-shrink: 0
    color: var(--color-text-tertiary)
    transition: transform 0.18s ease, color 0.15s ease

    &--expanded
      transform: rotate(180deg)

    .clarify-block__header:hover &
      color: var(--color-text-primary)

  // ---- 作答区 ----
  &__body
    padding: 12px 14px
    border-top: 1px solid var(--glass-border)

  &__question
    display: flex
    align-items: baseline
    gap: 8px
    margin-bottom: 10px

  &__question-text
    color: var(--color-text-primary)
    line-height: 1.5
    font-weight: 600

  &__mode-hint
    flex-shrink: 0
    font-size: 11px
    color: var(--color-text-tertiary)

  &__options
    display: flex
    flex-direction: column
    gap: 6px
    margin-bottom: 10px

  &__option
    display: flex
    align-items: center
    gap: 8px
    padding: 8px 10px
    border-radius: 10px
    border: 1px solid var(--glass-border)
    cursor: pointer
    transition: border-color 0.15s ease, background 0.15s ease
    user-select: none

    &:hover
      border-color: color-mix(in srgb, var(--color-accent) 45%, transparent)
      background: color-mix(in srgb, var(--color-accent) 5%, transparent)

    &--selected
      border-color: var(--color-accent)
      background: color-mix(in srgb, var(--color-accent) 9%, transparent)

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
      border-color: var(--color-accent)
      background: var(--color-accent)

  &__option-tick
    font-size: 11px
    line-height: 1
    color: var(--color-on-accent)

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
    color: var(--color-accent)
    border: 1px solid color-mix(in srgb, var(--color-accent) 45%, transparent)
    background: color-mix(in srgb, var(--color-accent) 10%, transparent)

  &__other
    width: 100%
    box-sizing: border-box
    padding: 8px 10px
    margin-bottom: 12px
    border-radius: 10px
    border: 1px solid var(--glass-border)
    background: transparent
    color: var(--color-text-primary)
    font-size: 13px
    outline: none
    transition: border-color 0.15s ease

    &:focus
      border-color: var(--color-accent)

    &::placeholder
      color: var(--color-text-tertiary)

  &__nav
    display: flex
    justify-content: flex-end
    gap: 8px

  &__btn
    padding: 6px 16px
    border-radius: 8px
    font-size: 12px
    font-weight: 500
    cursor: pointer
    transition: background 0.15s ease, border-color 0.15s ease, opacity 0.15s ease
    white-space: nowrap

    &:disabled
      opacity: 0.4
      cursor: not-allowed

    &--nav
      background: transparent
      border: 1px solid var(--glass-border)
      color: var(--color-text-secondary)

      &:hover:not(:disabled)
        border-color: color-mix(in srgb, var(--color-accent) 45%, transparent)
        background: color-mix(in srgb, var(--color-accent) 8%, transparent)

    &--finish
      background: var(--color-accent)
      border: 1px solid var(--color-accent)
      color: var(--color-on-accent)

      &:hover:not(:disabled)
        background: var(--color-accent-hover)
        border-color: var(--color-accent-hover)

  // ---- 完成态问答记录 ----
  &__qa-list
    display: flex
    flex-direction: column
    gap: 10px
    padding: 10px 14px 12px
    border-top: 1px solid var(--glass-border)

  &__q
    font-size: 12px
    color: var(--color-text-tertiary)
    line-height: 1.4

  &__a
    margin-top: 2px
    font-size: 13px
    color: var(--color-text-primary)
    line-height: 1.5

  // ---- 取消态 ----
  &__closed-icon
    flex-shrink: 0
    font-size: 12px
    color: var(--color-text-tertiary)

  &__summary
    color: var(--color-text-secondary)
</style>
