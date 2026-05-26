<template>
  <AppPageHero :kicker="kicker" :title="title" :subtitle="subtitle" feature>
    <template v-if="$slots.actions" #actions>
      <slot name="actions" />
    </template>
  </AppPageHero>
</template>

<script setup lang="ts">
import AppPageHero from "../layout/AppPageHero.vue";

defineProps<{
  kicker: string;
  title: string;
  subtitle?: string;
}>();
</script>
