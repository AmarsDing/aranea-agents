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
    style: { backgroundColor: 'var(--color-accent)', color: 'var(--color-on-accent, #fff)' },
  },
  ecosystem_preset: {
    label: '预设',
    style: { backgroundColor: 'var(--color-positive)', color: 'var(--color-on-accent, #fff)' },
  },
  marketplace: {
    label: '商城',
    style: { backgroundColor: 'var(--color-accent-indigo, #4F46E5)', color: 'var(--color-on-accent, #fff)' },
  },
  certified: {
    label: '认证',
    style: { backgroundColor: 'var(--color-warning, #F09B54)', color: 'var(--color-on-accent, #fff)' },
  },
};

const badgeConfig = computed(() => (props.kind ? (kindMap[props.kind] ?? null) : null));
</script>
