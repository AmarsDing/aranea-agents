<template>
  <q-card flat :class="['app-glass-panel', dense && 'app-glass-panel--dense']">
    <slot />
  </q-card>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    dense?: boolean;
  }>(),
  { dense: false },
);
</script>
