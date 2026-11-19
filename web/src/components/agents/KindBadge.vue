<template>
  <q-badge v-if="badgeConfig" :label="badgeConfig.label" class="q-ml-xs" :class="`kind-badge--${badgeConfig.tone}`" />
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps<{
  kind?: string;
}>();

const kindMap: Record<string, { label: string; tone: string }> = {
  system_builtin: { label: '内置', tone: 'builtin' },
  ecosystem_preset: { label: '预设', tone: 'preset' },
  marketplace: { label: '商城', tone: 'marketplace' },
  certified: { label: '认证', tone: 'certified' },
};

const badgeConfig = computed(() => (props.kind ? (kindMap[props.kind] ?? null) : null));
</script>

<style scoped lang="sass">
.kind-badge--builtin
  background: var(--color-accent)
  color: var(--color-on-accent)

.kind-badge--preset
  background: var(--color-success)
  color: var(--color-on-accent)

.kind-badge--marketplace
  background: var(--color-accent-indigo)
  color: var(--color-on-accent)

.kind-badge--certified
  background: var(--color-warning)
  color: var(--color-on-accent)
</style>
