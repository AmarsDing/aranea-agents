<template>
  <q-badge v-if="badgeConfig" :style="badgeConfig.style" :label="badgeConfig.label" class="q-ml-xs" />
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps<{
  kind?: string;
}>();

const kindMap: Record<string, { label: string; style: { backgroundColor: string; color: string } }> = {
  system_builtin: {
    label: '内置',
    style: { backgroundColor: 'var(--color-accent)', color: 'var(--color-on-accent)' },
  },
  ecosystem_preset: {
    label: '预设',
    style: { backgroundColor: 'var(--color-success)', color: 'var(--color-on-accent)' },
  },
  marketplace: {
    label: '商城',
    style: { backgroundColor: 'var(--color-accent-indigo)', color: 'var(--color-on-accent)' },
  },
  certified: {
    label: '认证',
    style: { backgroundColor: 'var(--color-warning)', color: 'var(--color-on-accent)' },
  },
};

const badgeConfig = computed(() => (props.kind ? (kindMap[props.kind] ?? null) : null));
</script>
