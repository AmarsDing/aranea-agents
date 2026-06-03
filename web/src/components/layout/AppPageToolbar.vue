<template>
  <q-card flat :bordered="variant === 'entity'" :class="toolbarClass">
    <q-card-section :class="['app-page-toolbar__body', { 'app-page-toolbar__body--dense': dense }]">
      <slot />
      <div v-if="$slots.actions" class="app-page-toolbar__actions app-actions-bar app-actions-bar--start">
        <slot name="actions" />
      </div>
    </q-card-section>
    <template v-if="$slots.footer">
      <q-separator v-if="stacked" />
      <q-card-section :class="['app-page-toolbar__footer', { 'app-page-toolbar__footer--dense': dense }]">
        <slot name="footer" />
      </q-card-section>
    </template>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(
  defineProps<{
    /** registry：标准列表页；entity：Agents / Team 等实体页（圆角更大、带边框） */
    variant?: 'registry' | 'entity';
    /** 实体页 Hero 下方间距 */
    offset?: boolean;
    /** 紧凑内边距 */
    dense?: boolean;
    /** footer 区块与主栏分隔 */
    stacked?: boolean;
    /** 暗色实体页 */
    isDark?: boolean;
  }>(),
  {
    variant: 'registry',
    offset: false,
    dense: false,
    stacked: false,
    isDark: false,
  },
);

const toolbarClass = computed(() => [
  'app-page-toolbar',
  `app-page-toolbar--${props.variant}`,
  {
    'app-page-toolbar--offset': props.offset,
    'is-dark': props.isDark,
  },
]);
</script>
