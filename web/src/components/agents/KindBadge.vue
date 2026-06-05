<template>
  <q-badge v-if="badgeConfig" :color="badgeConfig.color" :label="badgeConfig.label" class="q-ml-xs" />
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  kind: string
}>()

const kindMap: Record<string, { label: string; color: string }> = {
  system_builtin: { label: '内置', color: 'blue' },
  ecosystem_preset: { label: '预设', color: 'green' },
  marketplace: { label: '商城', color: 'purple' },
  certified: { label: '认证', color: 'orange' },
}

const badgeConfig = computed(() => kindMap[props.kind] ?? null)
</script>
