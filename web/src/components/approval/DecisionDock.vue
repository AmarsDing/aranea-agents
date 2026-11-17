<template>
  <section
    class="decision-dock"
    :class="{ 'decision-dock--batch': batchMode, 'decision-dock--busy': busy }"
    role="region"
    :aria-label="t('shell.approvalDock.ariaLabel')"
    :aria-busy="busy"
  >
    <div class="decision-dock__context">
      <template v-if="batchMode">
        <span class="decision-dock__mode">{{ t('shell.approvalDock.batchMode') }}</span>
        <strong class="decision-dock__count">{{ t('shell.approvalDock.selectedCount', { n: selectedCount }) }}</strong>
        <button class="decision-dock__link" type="button" @click="emit('clear-selection')">
          {{ t('shell.approvalDock.clearSelection') }}
        </button>
      </template>
      <template v-else-if="item">
        <span class="decision-dock__mode">{{ t('shell.approvalDock.singleMode') }}</span>
        <strong class="decision-dock__title">{{ item.title || item.id }}</strong>
        <span class="decision-dock__id mono">{{ item.id }}</span>
      </template>
      <span v-else class="decision-dock__hint">{{ t('shell.approvalDock.noSelection') }}</span>
    </div>

    <div class="decision-dock__reason">
      <label class="decision-dock__label" for="decision-dock-note">{{ t('shell.approvalDock.noteLabel') }}</label>
      <textarea
        id="decision-dock-note"
        ref="noteEl"
        class="decision-dock__textarea mono"
        rows="2"
        :value="note"
        :placeholder="t('shell.approvalDock.notePlaceholder')"
        :disabled="busy || (!item && !batchMode)"
        @input="onNoteInput"
      />
      <div class="decision-dock__templates">
        <span class="decision-dock__templates-label">{{ t('shell.approvalDock.reasonTemplates') }}</span>
        <button
          v-for="template in reasonTemplates"
          :key="template.label"
          class="decision-dock__chip"
          type="button"
          :disabled="busy"
          @click="applyTemplate(template.text)"
        >
          {{ template.label }}
        </button>
      </div>
    </div>

    <div class="decision-dock__actions">
      <template v-if="batchMode">
        <button
          class="decision-dock__btn decision-dock__btn--primary"
          type="button"
          :disabled="busy"
          @click="emit('batch', 'approve')"
        >
          {{ t('shell.approvalDock.approve') }}
        </button>
        <button
          class="decision-dock__btn decision-dock__btn--danger"
          type="button"
          :disabled="busy"
          @click="emit('batch', 'reject')"
        >
          {{ t('shell.approvalDock.reject') }}
        </button>
      </template>
      <template v-else>
        <button
          class="decision-dock__btn decision-dock__btn--primary"
          type="button"
          :disabled="busy || !item"
          aria-keyshortcuts="A"
          @click="emit('decide', 'approve')"
        >
          {{ t('shell.approvalDock.approve') }}
        </button>
        <button
          class="decision-dock__btn decision-dock__btn--danger"
          type="button"
          :disabled="busy || !item"
          aria-keyshortcuts="R"
          @click="emit('decide', 'reject')"
        >
          {{ t('shell.approvalDock.reject') }}
        </button>
        <button
          v-if="item?.kind === 'task_review'"
          class="decision-dock__btn decision-dock__btn--ghost"
          type="button"
          :disabled="busy || !item"
          @click="emit('decide', 'archive')"
        >
          {{ t('shell.approvalDock.archive') }}
        </button>
        <button
          class="decision-dock__btn decision-dock__btn--ghost"
          type="button"
          :disabled="busy || !item"
          aria-keyshortcuts="J"
          @click="emit('next')"
        >
          {{ t('shell.approvalDock.next') }}
        </button>
      </template>
      <span v-if="busy" class="decision-dock__busy" role="status">{{ t('shell.approvalDock.submitting') }}</span>
    </div>

    <div v-if="batchRunning" class="decision-dock__progress" role="status">
      <div class="decision-dock__bar">
        <i :style="{ width: `${progressPercent}%` }" />
      </div>
      <span class="mono">{{ t('shell.approvalDock.batchProgress', { done: batchDone, total: batchTotal }) }}</span>
    </div>

    <div v-if="outcomes.length && !batchRunning" class="decision-dock__results" role="status" aria-live="polite">
      <div class="decision-dock__results-head">
        <strong>{{ t('shell.approvalDock.resultsTitle') }}</strong>
        <span class="decision-dock__counts mono">
          {{ t('shell.approvalDock.batchDone', { ok: doneCount, conflict: conflictCount, failed: failedCount }) }}
        </span>
        <button class="decision-dock__link" type="button" @click="emit('dismiss-results')">
          {{ t('shell.approvalDock.dismissResults') }}
        </button>
      </div>

      <div v-if="failedOutcomes.length" class="decision-dock__failed">
        <span class="decision-dock__failed-title">{{ t('shell.approvalDock.failedTitle') }}</span>
        <ul class="decision-dock__failed-list">
          <li v-for="outcome in failedOutcomes" :key="outcome.key">
            <span class="decision-dock__failed-item">{{ outcome.title }}</span>
            <span class="decision-dock__failed-msg mono">{{ outcome.message || '—' }}</span>
          </li>
        </ul>
        <button class="decision-dock__btn decision-dock__btn--ghost" type="button" @click="emit('retry-failed')">
          {{ t('shell.approvalDock.retryFailed', { n: failedOutcomes.length }) }}
        </button>
      </div>

      <div v-if="conflictOutcomes.length" class="decision-dock__conflicts">
        {{ t('shell.approvalDock.conflictsTitle') }}：{{ conflictTitles }}
      </div>
    </div>

    <div class="decision-dock__keys">{{ t('shell.approvalDock.keyboardHint') }}</div>
  </section>
</template>

<script setup lang="ts">
/**
 * DecisionDock：审批页常驻裁决坞（业务专用控件，非普通按钮堆叠）。
 *
 * 交互合同：
 * - 单条模式：通过 / 驳回（理由必填）/ 归档（仅 task_review）/ 下一条；
 * - 批量模式（已勾选 > 0）：批量通过 / 批量驳回（共用一条理由）/ 清除选择；
 * - 处理中：aria-busy + 进度条 + 按钮禁用；
 * - 已处理 / 冲突（409）/ 失败逐条列出，失败项可只重试失败项；
 * - 键盘：由页面统一处理 A/R/J/K/X/Ctrl+A/Esc，按钮标注 aria-keyshortcuts。
 */
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { DecisionOutcome } from '../../features/approval/useApprovalsPage';
import type { ApprovalItemRow, DecideAction } from '../../features/approval/types';

const props = defineProps<{
  item: ApprovalItemRow | null;
  note: string;
  deciding: boolean;
  selectedCount: number;
  batchRunning: boolean;
  batchDone: number;
  batchTotal: number;
  outcomes: DecisionOutcome[];
}>();

const emit = defineEmits<{
  (event: 'update:note', value: string): void;
  (event: 'decide', action: DecideAction): void;
  (event: 'batch', action: DecideAction): void;
  (event: 'clear-selection'): void;
  (event: 'next'): void;
  (event: 'retry-failed'): void;
  (event: 'dismiss-results'): void;
}>();

const { t } = useI18n();
const noteEl = ref<HTMLTextAreaElement | null>(null);

const batchMode = computed(() => props.selectedCount > 0);
const busy = computed(() => props.deciding || props.batchRunning);
const progressPercent = computed(() =>
  props.batchTotal > 0 ? Math.round((props.batchDone / props.batchTotal) * 100) : 0,
);

const doneCount = computed(() => props.outcomes.filter((entry) => entry.status === 'done').length);
const conflictOutcomes = computed(() => props.outcomes.filter((entry) => entry.status === 'conflict'));
const failedOutcomes = computed(() => props.outcomes.filter((entry) => entry.status === 'failed'));
const conflictCount = computed(() => conflictOutcomes.value.length);
const failedCount = computed(() => failedOutcomes.value.length);
const conflictTitles = computed(() => conflictOutcomes.value.map((entry) => entry.title).join('、'));

const reasonTemplates = computed(() => [
  { label: t('shell.approvalDock.reasonInsufficient'), text: t('shell.approvalDock.reasonInsufficient') },
  { label: t('shell.approvalDock.reasonRisk'), text: t('shell.approvalDock.reasonRisk') },
  { label: t('shell.approvalDock.reasonMismatch'), text: t('shell.approvalDock.reasonMismatch') },
]);

function onNoteInput(event: Event) {
  emit('update:note', (event.target as HTMLTextAreaElement).value);
}

function applyTemplate(text: string) {
  const current = props.note.trim();
  emit('update:note', current ? `${current}；${text}` : text);
}

function focusNote() {
  noteEl.value?.focus();
}

defineExpose({ focusNote });
</script>

<style scoped lang="sass">
.decision-dock
  display: grid
  gap: var(--nt-space-3) var(--nt-space-4)
  grid-template-columns: minmax(0, 1fr)
  padding: var(--nt-space-3) var(--nt-space-4)
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-l)
  background: var(--nt-data-bg, var(--nt-bg-1))
  box-shadow: var(--nt-shadow-depth)
  color: var(--nt-text-1)

.decision-dock--batch
  border-color: var(--nt-accent-border)

@media (width >= 1100px)
  .decision-dock
    grid-template-columns: minmax(200px, 1fr) minmax(280px, 1.6fr) auto
    align-items: end

  .decision-dock__progress,
  .decision-dock__results,
  .decision-dock__keys
    grid-column: 1 / -1

.decision-dock__context
  display: flex
  flex-direction: column
  gap: var(--nt-space-1)
  min-width: 0

.decision-dock__mode
  font-size: var(--nt-fs-caption)
  color: var(--nt-text-2)
  letter-spacing: 0.04em
  text-transform: uppercase

.decision-dock__title,
.decision-dock__count
  color: var(--nt-text-0)
  font-size: var(--nt-fs-panel)
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

.decision-dock__id
  font-size: var(--nt-fs-caption)
  color: var(--nt-text-2)
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

.decision-dock__hint
  color: var(--nt-text-2)
  font-size: var(--nt-fs-body)

.decision-dock__reason
  display: flex
  flex-direction: column
  gap: var(--nt-space-2)
  min-width: 0

.decision-dock__label
  font-size: var(--nt-fs-caption)
  color: var(--nt-text-2)

.decision-dock__textarea
  width: 100%
  resize: vertical
  min-height: 44px
  padding: var(--nt-space-2) var(--nt-space-3)
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-m)
  background: var(--nt-bg-2)
  color: var(--nt-text-0)
  font-size: var(--nt-fs-body)
  line-height: 1.4

  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
    border-color: var(--nt-accent-border)

.decision-dock__templates
  display: flex
  flex-wrap: wrap
  align-items: center
  gap: var(--nt-space-2)

.decision-dock__templates-label
  font-size: var(--nt-fs-caption)
  color: var(--nt-text-2)

.decision-dock__chip
  padding: var(--nt-space-1) var(--nt-space-3)
  border: 1px solid var(--nt-border-2)
  border-radius: var(--nt-radius-pill)
  background: transparent
  color: var(--nt-text-1)
  font-size: var(--nt-fs-caption)
  cursor: pointer

  &:hover
    border-color: var(--nt-accent-border)
    color: var(--nt-accent)

  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px

.decision-dock__actions
  display: flex
  flex-wrap: wrap
  align-items: center
  gap: var(--nt-space-2)
  justify-content: flex-end

.decision-dock__btn
  min-height: 36px
  padding: 0 var(--nt-space-4)
  border-radius: var(--nt-radius-m)
  border: 1px solid transparent
  font-size: var(--nt-fs-panel)
  font-weight: 600
  cursor: pointer
  transition: background 120ms ease, border-color 120ms ease, color 120ms ease

  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px

  &:disabled
    cursor: not-allowed
    opacity: 0.45

.decision-dock__btn--primary
  background: var(--nt-accent)
  color: var(--nt-on-accent)

  &:not(:disabled):hover
    background: color-mix(in srgb, var(--nt-accent) 82%, var(--nt-ink))

.decision-dock__btn--danger
  background: transparent
  border-color: var(--nt-danger)
  color: var(--nt-danger)

  &:not(:disabled):hover
    background: color-mix(in srgb, var(--nt-danger) 14%, transparent)

.decision-dock__btn--ghost
  background: transparent
  border-color: var(--nt-border-2)
  color: var(--nt-text-1)

  &:not(:disabled):hover
    border-color: var(--nt-accent-border)
    color: var(--nt-accent)

@media (pointer: coarse)
  .decision-dock__btn
    min-height: 44px

.decision-dock__busy
  font-size: var(--nt-fs-caption)
  color: var(--nt-text-2)

.decision-dock__progress
  display: flex
  align-items: center
  gap: var(--nt-space-3)
  font-size: var(--nt-fs-caption)
  color: var(--nt-text-2)

.decision-dock__bar
  flex: 1
  height: 4px
  border-radius: var(--nt-radius-pill)
  background: var(--nt-bg-3)
  overflow: hidden

  i
    display: block
    height: 100%
    background: var(--nt-accent)
    transition: width 160ms ease

.decision-dock__results
  display: flex
  flex-direction: column
  gap: var(--nt-space-2)
  padding-top: var(--nt-space-2)
  border-top: 1px solid var(--nt-border-1)
  font-size: var(--nt-fs-caption)

.decision-dock__results-head
  display: flex
  flex-wrap: wrap
  align-items: center
  gap: var(--nt-space-3)
  color: var(--nt-text-1)

.decision-dock__counts
  color: var(--nt-text-2)

.decision-dock__failed
  display: flex
  flex-direction: column
  gap: var(--nt-space-2)

.decision-dock__failed-title
  color: var(--nt-danger)

.decision-dock__failed-list
  margin: 0
  padding-left: var(--nt-space-4)
  display: flex
  flex-direction: column
  gap: var(--nt-space-1)

  li
    display: flex
    gap: var(--nt-space-2)
    flex-wrap: wrap

.decision-dock__failed-item
  color: var(--nt-text-0)

.decision-dock__failed-msg
  color: var(--nt-text-2)

.decision-dock__conflicts
  color: var(--nt-text-2)

.decision-dock__link
  padding: 0
  border: none
  background: transparent
  color: var(--nt-accent)
  font-size: var(--nt-fs-caption)
  cursor: pointer
  text-decoration: underline

  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px

.decision-dock__keys
  font-size: var(--nt-fs-caption)
  color: var(--nt-text-2)
  font-family: var(--nt-font-mono)

.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums
</style>
