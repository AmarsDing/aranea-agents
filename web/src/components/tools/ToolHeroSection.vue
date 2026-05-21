<template>
  <section class="app-page-hero">
    <div>
      <div class="app-page-kicker">{{ kicker }}</div>
      <h1 class="app-page-title">{{ title }}</h1>
      <p v-if="subtitle" class="app-page-subtitle">{{ subtitle }}</p>
    </div>
    <div v-if="$slots.actions" class="row q-gutter-sm">
      <slot name="actions" />
    </div>
  </section>
</template>

<script setup lang="ts">
defineProps<{
  kicker: string;
  title: string;
  subtitle?: string;
}>();
</script>
