<template>
  <span
    class="app-registry-hover-tip"
    :class="{ 'app-registry-hover-tip--active': showTip, 'app-registry-hover-tip--block': block }"
  >
    <slot />
    <q-icon v-if="showTip && indicator" name="info_outline" class="app-registry-hover-tip__icon" aria-hidden="true" />
    <q-tooltip
      v-if="showTip"
      anchor="top start"
      self="bottom start"
      :offset="[0, 10]"
      content-class="app-registry-hover-tip__popup"
    >
      <div class="app-registry-hover-tip__content">{{ displayText }}</div>
    </q-tooltip>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(
  defineProps<{
    /** 悬停展示的完整描述；空则不显示 tooltip */
    text?: string | null;
    /** text 为空时在 tooltip 中显示的占位文案；设为 false 则不显示 tooltip */
    emptyLabel?: string | false;
    /** 有内容时在单元格末尾显示提示图标 */
    indicator?: boolean;
    /** 块级包裹（占满单元格宽度，便于整格悬停） */
    block?: boolean;
  }>(),
  {
    text: '',
    emptyLabel: false,
    indicator: true,
    block: true,
  },
);

const normalized = computed(() => String(props.text ?? '').trim());

const showTip = computed(() => normalized.value.length > 0 || props.emptyLabel !== false);

const displayText = computed(() => normalized.value || (props.emptyLabel === false ? '' : props.emptyLabel || ''));
</script>
