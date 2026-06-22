<template>
  <section :class="['app-page-hero', { 'app-page-hero--feature': feature }]">
    <div>
      <div v-if="kicker" class="app-page-kicker">{{ kicker }}</div>
      <h1 class="app-page-title">{{ title }}</h1>
      <p v-if="subtitle" class="app-page-subtitle">{{ subtitle }}</p>
      <slot />
    </div>
    <div v-if="$slots.actions" class="app-actions-bar row q-gutter-sm items-center">
      <slot name="actions" />
    </div>
  </section>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    kicker?: string;
    title: string;
    subtitle?: string;
    feature?: boolean;
  }>(),
  {
    feature: false,
  },
);
</script>
