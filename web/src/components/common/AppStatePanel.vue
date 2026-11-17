<template>
  <div
    class="app-state-panel"
    :class="[`app-state-panel--${variant}`, { 'app-state-panel--dense': dense }]"
    :data-state="variant"
    data-test="app-state-panel"
    :role="isAlert ? 'alert' : 'status'"
    :aria-live="isAlert ? 'assertive' : 'polite'"
    :aria-busy="variant === 'loading' ? 'true' : 'false'"
  >
    <div v-if="variant === 'loading'" class="app-state-panel__skeleton" aria-hidden="true">
      <q-skeleton
        v-for="row in rows"
        :key="row"
        type="text"
        :class="row === 1 ? 'app-state-panel__skeleton-lead' : ''"
      />
    </div>
    <template v-else>
      <div class="app-state-panel__icon" aria-hidden="true">
        <q-icon :name="resolvedIcon" size="26px" />
      </div>
      <div class="app-state-panel__copy">
        <p class="app-state-panel__title">{{ resolvedTitle }}</p>
        <p v-if="resolvedDescription" class="app-state-panel__desc">{{ resolvedDescription }}</p>
      </div>
      <div v-if="hasActions || $slots.actions" class="app-state-panel__actions">
        <slot name="actions">
          <q-btn
            v-if="actionLabel"
            unelevated
            no-caps
            color="primary"
            :icon="actionIcon"
            :label="actionLabel"
            @click="emit('action')"
          />
          <q-btn v-if="secondaryLabel" flat no-caps :label="secondaryLabel" @click="emit('secondary')" />
        </slot>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';

type StateVariant = 'loading' | 'empty' | 'error' | 'partial' | 'forbidden';

const props = withDefaults(
  defineProps<{
    variant?: StateVariant;
    title?: string;
    description?: string;
    icon?: string;
    actionLabel?: string;
    actionIcon?: string;
    secondaryLabel?: string;
    /** 紧凑模式：表格/卡片内部使用，减少上下留白 */
    dense?: boolean;
    /** loading 骨架行数 */
    rows?: number;
  }>(),
  {
    variant: 'empty',
    title: '',
    description: '',
    icon: '',
    actionLabel: '',
    actionIcon: '',
    secondaryLabel: '',
    dense: false,
    rows: 3,
  },
);

const emit = defineEmits<{ action: []; secondary: [] }>();
const { t } = useI18n();

const isAlert = computed(
  () => props.variant === 'error' || props.variant === 'partial' || props.variant === 'forbidden',
);

const DEFAULT_ICONS: Record<StateVariant, string> = {
  loading: 'hourglass_empty',
  empty: 'inbox',
  error: 'error_outline',
  partial: 'warning_amber',
  forbidden: 'lock',
};

const resolvedIcon = computed(() => props.icon || DEFAULT_ICONS[props.variant]);

const resolvedTitle = computed(() => {
  if (props.title) return props.title;
  if (props.variant === 'empty') return t('shell.state.emptyTitle');
  if (props.variant === 'error') return t('shell.state.errorTitle');
  if (props.variant === 'partial') return t('shell.state.partialTitle');
  if (props.variant === 'forbidden') return t('shell.state.forbiddenTitle');
  return t('shell.state.loading');
});

const resolvedDescription = computed(() => {
  if (props.description) return props.description;
  if (props.variant === 'empty') return t('shell.state.emptyDesc');
  if (props.variant === 'error') return t('shell.state.errorDesc');
  if (props.variant === 'partial') return t('shell.state.partialDesc');
  if (props.variant === 'forbidden') return t('shell.state.forbiddenDesc');
  return '';
});

const hasActions = computed(() => Boolean(props.actionLabel || props.secondaryLabel));
</script>
