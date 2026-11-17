<template>
  <q-dialog
    v-model="open"
    :persistent="persistent || loading"
    :position="dialogPosition"
    :maximized="maximized"
    no-refocus
    @escape-key="requestClose"
    @hide="restoreFocus"
  >
    <q-card
      class="app-dialog"
      :class="cardClasses"
      :aria-labelledby="titleId"
      :aria-busy="loading"
    >
      <q-card-section class="app-dialog__header">
        <div class="app-dialog__header-main">
          <slot name="header-prefix" />
          <div class="app-dialog__heading">
            <q-icon v-if="icon" :name="icon" size="22px" aria-hidden="true" />
            <div class="app-dialog__heading-copy">
              <h2 :id="titleId" class="app-dialog__title">{{ title }}</h2>
              <p v-if="subtitle" class="app-dialog__subtitle">{{ subtitle }}</p>
            </div>
          </div>
        </div>
        <q-btn
          flat
          round
          dense
          icon="close"
          data-test="dialog-close"
          :disable="loading"
          :aria-label="t('shell.common.close')"
          @click="closeExplicit"
        />
      </q-card-section>
      <q-card-section class="app-dialog__body"><slot /></q-card-section>
      <q-card-actions v-if="$slots.actions" align="right" class="app-dialog__actions">
        <slot name="actions" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, watch } from 'vue';
import { useI18n } from 'vue-i18n';

const props = withDefaults(
  defineProps<{
    modelValue: boolean;
    title: string;
    /** 标题下的单行补充说明（如对象名、统计信息）；不传则不渲染。 */
    subtitle?: string;
    size?: 'sm' | 'md' | 'lg' | 'xl';
    icon?: string;
    loading?: boolean;
    persistent?: boolean;
    /** 抽屉定位；非 standard 时卡片撑满视口高度并去圆角（右侧/左侧抽屉）。 */
    position?: 'standard' | 'top' | 'right' | 'bottom' | 'left';
    /** 全屏弹层：铺满视口，忽略 size 宽度与常规最大高度。 */
    maximized?: boolean;
    /** 追加到卡片的领域类名，仅用于迁移期承接既有作用域样式。 */
    cardClass?: string;
  }>(),
  {
    size: 'md',
    icon: '',
    subtitle: '',
    loading: false,
    persistent: false,
    position: 'standard',
    maximized: false,
    cardClass: '',
  },
);

const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>();
const { t } = useI18n();
const titleId = `app-dialog-title-${Math.random().toString(36).slice(2)}`;
let returnFocusTo: HTMLElement | null = null;

const open = computed({
  get: () => props.modelValue,
  set: (value: boolean) => {
    if (!value && props.loading) return;
    emit('update:modelValue', value);
  },
});

const isDrawer = computed(() => props.position === 'right' || props.position === 'left');
const dialogPosition = computed(() => (props.position === 'standard' ? undefined : props.position));
const cardClasses = computed(() => [
  `app-dialog--${props.size}`,
  {
    'app-dialog--drawer': isDrawer.value,
    [`app-dialog--drawer-${props.position}`]: isDrawer.value,
    'app-dialog--maximized': props.maximized,
  },
  props.cardClass,
]);

/** Esc / 遮罩视为隐式关闭：loading 与 persistent 都拦截（见 08-重设计方案 §8.0.3）。 */
function requestClose() {
  if (props.loading || props.persistent) return;
  open.value = false;
}

/** 标题栏关闭按钮是显式关闭：persistent 仍可关闭，仅 loading 时禁用。 */
function closeExplicit() {
  if (props.loading) return;
  open.value = false;
}

async function restoreFocus() {
  await nextTick();
  returnFocusTo?.focus({ preventScroll: true });
}

function captureFocusTarget() {
  const active = document.activeElement;
  if (!(active instanceof HTMLElement)) return null;
  if (active.classList.contains('q-focus-helper')) {
    return active.closest<HTMLElement>('.q-btn, button, a');
  }
  return active;
}

watch(
  () => props.modelValue,
  async (visible, wasVisible) => {
    if (visible) {
      returnFocusTo = captureFocusTarget();
    } else if (wasVisible) {
      await restoreFocus();
    }
  },
  { immediate: true, flush: 'sync' },
);

onBeforeUnmount(() => returnFocusTo?.focus());
</script>
