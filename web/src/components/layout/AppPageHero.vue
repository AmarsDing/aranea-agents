<template>
  <!--
    页头压成一行：标题在左、主操作在右。kicker（英文小字分类）不再显示；
    副标题收进标题旁的说明图标，悬停或键盘聚焦时浮出，不再占一整行。
  -->
  <section :class="['app-page-hero', { 'app-page-hero--feature': feature }]">
    <div class="app-page-hero__main">
      <h1 class="app-page-title">{{ title }}</h1>
      <span v-if="subtitle" class="app-page-hero__hint" tabindex="0" :aria-label="subtitle">
        <q-icon name="info_outline" size="18px" aria-hidden="true" />
        <q-tooltip anchor="bottom left" self="top left" :offset="[0, 6]" max-width="420px">{{ subtitle }}</q-tooltip>
      </span>
      <slot />
    </div>
    <div v-if="$slots.actions" class="app-page-hero__actions app-actions-bar">
      <slot name="actions" />
    </div>
  </section>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    /** 保留入参以兼容既有调用；紧凑页头不再显示。 */
    kicker?: string;
    title: string;
    subtitle?: string;
    feature?: boolean;
  }>(),
  {
    kicker: undefined,
    subtitle: undefined,
    feature: false,
  },
);
</script>
