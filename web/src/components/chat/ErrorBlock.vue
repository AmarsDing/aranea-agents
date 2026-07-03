<template>
  <div class="error-block error-block--degradation">
    <div class="error-block__content">
      <span class="error-block__icon">⚠️</span>
      <span class="error-block__message">{{ step.Content }}</span>
      <span v-if="hintLabel" class="error-block__hint">（{{ hintLabel }}）</span>
      <span v-if="step.ToolErrorCode" class="error-block__code">{{ step.ToolErrorCode }}</span>
    </div>
    <div v-if="action !== 'none'" class="error-block__actions">
      <q-btn
        v-if="action === 'retry'"
        flat
        dense
        size="sm"
        :label="t('chat.errorBlock.btnRetry')"
        class="error-block__btn"
        @click="onRetry"
      />
      <q-btn
        v-else-if="action === 'switch_model'"
        flat
        dense
        size="sm"
        :label="t('chat.errorBlock.btnSwitchModel')"
        class="error-block__btn"
        @click="onSwitchModel"
      />
      <q-btn
        v-else-if="action === 'rephrase'"
        flat
        dense
        size="sm"
        :label="t('chat.errorBlock.btnRephrase')"
        class="error-block__btn"
        @click="onRephrase"
      />
      <q-btn
        v-else-if="action === 'check_config'"
        flat
        dense
        size="sm"
        :label="t('chat.errorBlock.btnCheckConfig')"
        class="error-block__btn"
        @click="onCheckConfig"
      />
      <q-btn
        v-else-if="action === 'remove_attachment'"
        flat
        dense
        size="sm"
        :label="t('chat.errorBlock.btnRemoveAttachment')"
        class="error-block__btn"
        @click="onRemoveAttachment"
      />
      <q-btn
        v-else-if="action === 'relogin'"
        flat
        dense
        size="sm"
        :label="t('chat.errorBlock.btnRelogin')"
        class="error-block__btn"
        @click="onRelogin"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Step } from '../../features/chat/v2Types';
import { getErrorAction, getActionHintLabelKey, type ErrorAction } from '../../features/chat/errorCodeHints';

const { t } = useI18n();

const props = defineProps<{
  step: Step;
}>();

const emit = defineEmits<{
  /** User clicked "retry" — re-send the failed user message. */
  retry: [step: Step];
  /** User clicked "switch model" — open model picker / settings. */
  'switch-model': [step: Step];
  /** User clicked "rephrase" — focus composer for editing. */
  rephrase: [step: Step];
  /** User clicked "check config" — navigate to agent settings. */
  'check-config': [step: Step];
  /** User clicked "remove attachment" — drop the offending attachment. */
  'remove-attachment': [step: Step];
  /** User clicked "relogin" — redirect to login page. */
  relogin: [step: Step];
}>();

/** Resolved action for the current error code (falls back to `none`). */
const action = computed<ErrorAction>(() => getErrorAction(props.step.ToolErrorCode || undefined));

/** Inline hint label shown next to the message (e.g. "建议切换模型"). */
const hintLabel = computed(() => {
  const key = getActionHintLabelKey(action.value);
  if (!key) return '';
  return t(key);
});

function onRetry() {
  emit('retry', props.step);
}
function onSwitchModel() {
  emit('switch-model', props.step);
}
function onRephrase() {
  emit('rephrase', props.step);
}
function onCheckConfig() {
  emit('check-config', props.step);
}
function onRemoveAttachment() {
  emit('remove-attachment', props.step);
}
function onRelogin() {
  emit('relogin', props.step);
}
</script>

<style lang="sass" scoped>
.error-block
  display: flex
  flex-wrap: wrap
  align-items: center
  gap: 6px 10px
  padding: 6px 10px
  border-radius: 8px
  border-left: 3px solid var(--color-danger)
  font-size: 13px

  &--degradation
    background: var(--chat-status-danger-bg)
    color: var(--color-danger)

  &--info
    background: var(--glass-surface)
    border-left-color: var(--color-text-tertiary)
    color: var(--color-text-secondary)

  &__content
    display: flex
    align-items: center
    gap: 6px
    flex: 1
    min-width: 0

  &__icon
    font-size: 13px
    flex-shrink: 0

  &__message
    line-height: 1.4
    flex: 1
    min-width: 0

  &__hint
    font-size: 12px
    color: var(--color-text-secondary)
    flex-shrink: 0

  &__code
    font-size: 11px
    color: var(--color-text-tertiary)
    flex-shrink: 0

  &__actions
    display: flex
    align-items: center
    gap: 4px
    flex-shrink: 0

  &__btn
    :deep(.q-btn__content)
      font-size: 12px
    color: var(--color-accent)

    &:hover
      color: var(--color-accent-hover)
</style>
