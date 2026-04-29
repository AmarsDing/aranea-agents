<template>
  <q-card flat :class="['tool-glass-card', dense && 'tool-glass-card--dense']">
    <slot />
  </q-card>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    dense?: boolean;
  }>(),
  { dense: false }
);
</script>

<style scoped lang="sass">
.tool-glass-card
  border-radius: 18px
  border: 1px solid var(--glass-border)
  background: var(--glass-surface)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))
  box-shadow: none
  overflow: hidden

.tool-glass-card--dense :deep(.q-card__section)
  padding-top: 12px
  padding-bottom: 12px
</style>
