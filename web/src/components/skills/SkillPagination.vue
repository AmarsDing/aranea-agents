<template>
  <footer class="skill-pagination app-registry-pagination q-mt-md">
    <div class="skill-pagination__summary">{{ total }} {{ label }}</div>
    <div class="skill-pagination__controls row items-center no-wrap">
      <q-select
        :model-value="pageSize"
        dense
        outlined
        emit-value
        map-options
        label="每页"
        :options="pageSizeOptions"
        class="skill-pagination__page-size"
        @update:model-value="emit('update:pageSize', Number($event))"
      />
      <span class="skill-pagination__page-label">第 {{ page }} / {{ pageMax }} 页</span>
      <q-btn round dense flat icon="chevron_left" :disable="page <= 1 || loading" @click="emit('update:page', page - 1)" />
      <q-btn round dense flat icon="chevron_right" :disable="page >= pageMax || loading" @click="emit('update:page', page + 1)" />
    </div>
  </footer>
</template>

<script setup lang="ts">
defineProps<{
  page: number;
  pageSize: number;
  pageMax: number;
  total: number;
  loading?: boolean;
  label: string;
}>();

const emit = defineEmits<{
  "update:page": [value: number];
  "update:pageSize": [value: number];
}>();

const pageSizeOptions = [10, 20, 50].map((value) => ({ label: String(value), value }));
</script>

<style scoped lang="sass">
.skill-pagination
  border: 1px solid var(--glass-border)
  border-radius: 16px
  background: color-mix(in srgb, var(--glass-surface) 92%, transparent)
  padding: var(--space-3) var(--space-4)

.skill-pagination__summary
  font-size: var(--text-sm)
  color: var(--color-text-secondary)

.skill-pagination__controls
  gap: var(--space-2)

.skill-pagination__page-size
  min-width: 108px

.skill-pagination__page-label
  font-size: var(--text-xs)
  color: var(--color-text-secondary)
  white-space: nowrap
  padding: 0 var(--space-1)

@media (width <= 720px)
  .skill-pagination
    flex-direction: column
    align-items: stretch

  .skill-pagination__controls
    justify-content: space-between
</style>
