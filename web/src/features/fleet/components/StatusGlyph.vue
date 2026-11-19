<template>
  <span
    v-if="status && status !== 'shell'"
    class="fleet-glyph"
    :class="`is-${status}`"
    :title="label"
    role="img"
    :aria-label="label"
  >
    <q-spinner v-if="status === 'working' || status === 'starting' || status === 'creating'" size="12px" />
    <q-icon v-else-if="status === 'needs_input'" name="help" size="14px" />
    <span v-else class="fleet-glyph__dot" />
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { STATUS_TEXT } from '../useFleet';

const props = defineProps<{ status: string }>();
const label = computed(() => STATUS_TEXT[props.status] ?? props.status);
</script>

<style scoped lang="sass">
.fleet-glyph
  display: inline-flex
  align-items: center
  justify-content: center
  width: 16px
  height: 16px
  flex: none
  color: var(--nt-accent)
  &.is-needs_input
    color: var(--nt-warn)
  &.is-done .fleet-glyph__dot
    background: var(--nt-success)
  &.is-idle .fleet-glyph__dot, &.is-exited .fleet-glyph__dot
    background: var(--nt-text-3)
  &.is-failed .fleet-glyph__dot
    background: var(--nt-danger)
.fleet-glyph__dot
  width: 8px
  height: 8px
  border-radius: var(--nt-radius-pill)
  background: var(--nt-text-3)
</style>
