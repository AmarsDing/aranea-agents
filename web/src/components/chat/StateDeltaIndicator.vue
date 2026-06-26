<template>
  <div class="state-delta-indicator row items-center q-gutter-xs q-py-xs">
    <q-chip dense outline color="accent" size="sm">{{ delta.operation }}</q-chip>
    <code class="state-delta-indicator__path">{{ delta.path }}</code>
    <span v-if="delta.value_json" class="state-delta-indicator__value text-caption ellipsis">{{ previewValue }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps<{
  delta: { operation: string; path: string; value_json: string };
}>();

const previewValue = computed(() => {
  const raw = props.delta.value_json ?? '';
  return raw.length > 120 ? `${raw.slice(0, 120)}…` : raw;
});
</script>
