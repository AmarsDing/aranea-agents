<template>
  <section
    ref="rootRef"
    class="bulk-bar"
    role="region"
    :aria-label="t('shell.bulkBar.ariaLabel')"
    :aria-busy="busy ? 'true' : 'false'"
    tabindex="-1"
    data-test="bulk-bar"
    @keydown.esc.stop="onEscape"
  >
    <div class="bulk-bar__count" aria-live="polite">
      <q-icon name="check_circle" size="18px" aria-hidden="true" />
      <span>{{ t('shell.bulkBar.selected', { n: count }) }}</span>
      <span v-if="total" class="bulk-bar__total">/ {{ total }}</span>
    </div>

    <div v-if="error" class="bulk-bar__error" role="alert" data-test="bulk-bar-error">
      <q-icon name="error_outline" size="16px" aria-hidden="true" />
      <span class="bulk-bar__error-text">{{ error }}</span>
      <q-btn
        flat
        dense
        no-caps
        size="sm"
        data-test="bulk-retry"
        :label="t('shell.common.retry')"
        :disable="Boolean(busy)"
        @click="retry"
      />
    </div>

    <div v-if="pendingAction" class="bulk-bar__confirm" data-test="bulk-bar-confirm">
      <span class="bulk-bar__confirm-text">{{ pendingAction.confirmLabel || pendingAction.label }}</span>
      <q-btn
        flat
        dense
        no-caps
        size="sm"
        data-test="bulk-confirm-cancel"
        :label="t('shell.common.cancel')"
        @click="pendingKey = null"
      />
      <q-btn
        dense
        no-caps
        unelevated
        size="sm"
        data-test="bulk-confirm"
        :color="bulkActionColor(pendingAction.tone)"
        :label="t('shell.common.confirm')"
        @click="confirmPending"
      />
    </div>

    <div v-else class="bulk-bar__actions">
      <q-btn
        v-for="action in actions"
        :key="action.key"
        flat
        dense
        rounded
        no-caps
        :color="bulkActionColor(action.tone)"
        :icon="action.icon"
        :label="action.label"
        :disable="Boolean(busy) || action.disabled"
        :loading="busy === action.key"
        :data-test="`bulk-action-${action.key}`"
        @click="trigger(action)"
      >
        <q-tooltip v-if="action.hint">{{ action.hint }}</q-tooltip>
      </q-btn>
      <q-separator vertical class="bulk-bar__sep" />
      <q-btn
        flat
        dense
        round
        icon="close"
        data-test="bulk-clear"
        :aria-label="t('shell.bulkBar.clear')"
        :disable="Boolean(busy)"
        @click="$emit('clear')"
      />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { bulkActionColor, type BulkAction } from './bulkAction';

const props = withDefaults(
  defineProps<{
    /** 已选条目数。 */
    count: number;
    /** 当前列表总条目数，仅用于「已选 n / total」上下文。 */
    total?: number;
    actions: BulkAction[];
    /** 正在执行的动作键：全部按钮禁用，对应按钮 loading。 */
    busy?: string;
    /** 批量动作失败信息：就地 role=alert 展示并可重试，不退化为 toast。 */
    error?: string;
  }>(),
  { total: 0, busy: '', error: '' },
);

const emit = defineEmits<{ run: [key: string]; clear: [] }>();
const { t } = useI18n();

const rootRef = ref<HTMLElement | null>(null);
const pendingKey = ref<string | null>(null);
const lastKey = ref('');
const pendingAction = computed(() => props.actions.find((a) => a.key === pendingKey.value) ?? null);

function focusBar() {
  rootRef.value?.focus({ preventScroll: true });
}

onMounted(() => {
  if (props.count > 0) focusBar();
});

watch(
  () => props.count,
  async (next, prev) => {
    if (next > 0 && (prev ?? 0) === 0) {
      await nextTick();
      focusBar();
    }
  },
);

function trigger(action: BulkAction) {
  if (props.busy || action.disabled) return;
  lastKey.value = action.key;
  if (action.requiresConfirm) {
    pendingKey.value = action.key;
    return;
  }
  emit('run', action.key);
}

function confirmPending() {
  const key = pendingKey.value;
  pendingKey.value = null;
  if (key) emit('run', key);
}

function retry() {
  if (props.busy || !lastKey.value) return;
  emit('run', lastKey.value);
}

/** Esc 只回退一步：确认态先退回动作态，动作态才清除选择。 */
function onEscape() {
  if (pendingKey.value) {
    pendingKey.value = null;
    return;
  }
  emit('clear');
}
</script>

<style scoped lang="sass">
.bulk-bar
  position: sticky
  bottom: var(--nt-space-3)
  z-index: 4
  display: flex
  flex-wrap: wrap
  align-items: center
  gap: var(--nt-space-3)
  margin-top: var(--nt-space-3)
  padding: var(--nt-space-2) var(--nt-space-4)
  border: 1px solid var(--nt-border-2)
  border-radius: var(--nt-radius-l)
  background: var(--nt-data-raised)
  box-shadow: var(--nt-shadow-depth), var(--nt-glow-accent)
  color: var(--nt-text-1)

  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 4px

  &__count
    display: inline-flex
    align-items: center
    gap: var(--nt-space-1)
    color: var(--nt-text-0)
    font-size: var(--nt-fs-panel)
    font-weight: 650

  &__total
    color: var(--nt-text-3)
    font-family: var(--nt-font-mono)
    font-variant-numeric: tabular-nums

  &__actions
    display: flex
    align-items: center
    gap: var(--nt-space-1)
    margin-left: auto

  &__sep
    height: 18px
    margin: 0 var(--nt-space-1)

  &__confirm
    display: flex
    align-items: center
    gap: var(--nt-space-2)
    margin-left: auto

  &__confirm-text
    color: var(--nt-warn)
    font-size: var(--nt-fs-body)

  &__error
    display: flex
    align-items: center
    gap: var(--nt-space-2)
    margin-left: auto
    color: var(--nt-danger)
    font-size: var(--nt-fs-body)

  &__error-text
    max-width: 48ch
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

@media (max-width: 559px)
  .bulk-bar
    bottom: 0
    border-radius: var(--nt-radius-l) var(--nt-radius-l) 0 0

    &__actions,
    &__confirm,
    &__error
      margin-left: 0
</style>
