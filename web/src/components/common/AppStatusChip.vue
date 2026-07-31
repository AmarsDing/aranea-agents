<template>
  <span class="app-status-chip" :class="`app-status-chip--${tone}`">
    <q-icon :name="icon" size="13px" aria-hidden="true" />
    <span class="app-status-chip__label">{{ label }}</span>
    <q-tooltip v-if="tooltipText" anchor="top middle" self="bottom middle" :offset="[0, 8]">
      {{ tooltipText }}
    </q-tooltip>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { appStatusMeta } from '../../features/ui/appStatusMeta';

/**
 * 通用状态标签：内置常见状态（success/failed/timeout/...）的色调与多语言文案，
 * 未收录状态兜底显示原文。状态元数据见 `features/ui/appStatusMeta.ts`。
 */
const props = defineProps<{
  /** 原始状态值（success/failed/timeout/...） */
  status?: string;
  /** 悬停提示（如 error_message）；空则不显示 */
  tooltip?: string;
}>();

const { t } = useI18n();

const meta = computed(() => appStatusMeta(props.status));
const tone = computed(() => meta.value?.tone ?? 'neutral');
const icon = computed(() => meta.value?.icon ?? 'help_outline');
const label = computed(() => {
  if (meta.value) return t(meta.value.labelKey);
  const raw = (props.status ?? '').trim();
  return raw || t('common.status.unknown');
});
const tooltipText = computed(() => (props.tooltip ?? '').trim());
</script>

<style scoped lang="sass">
.app-status-chip
  display: inline-flex
  align-items: center
  gap: 4px
  padding: 2px 8px
  border-radius: 999px
  font-size: var(--text-xs)
  font-weight: 700
  line-height: 1.4
  white-space: nowrap
  border: 1px solid var(--glass-border)
  background: var(--glass-surface)

.app-status-chip--success
  color: var(--color-success)
  border-color: color-mix(in srgb, var(--color-success) 25%, var(--glass-border))
  background: color-mix(in srgb, var(--color-success) 8%, var(--glass-surface))

.app-status-chip--danger
  color: var(--color-danger)
  border-color: color-mix(in srgb, var(--color-danger) 25%, var(--glass-border))
  background: color-mix(in srgb, var(--color-danger) 8%, var(--glass-surface))

.app-status-chip--warning
  color: var(--color-warning)
  border-color: color-mix(in srgb, var(--color-warning) 25%, var(--glass-border))
  background: color-mix(in srgb, var(--color-warning) 8%, var(--glass-surface))

.app-status-chip--info
  color: var(--color-accent)
  border-color: color-mix(in srgb, var(--color-accent) 25%, var(--glass-border))
  background: color-mix(in srgb, var(--color-accent) 8%, var(--glass-surface))

.app-status-chip--neutral
  color: var(--color-text-secondary)
</style>
