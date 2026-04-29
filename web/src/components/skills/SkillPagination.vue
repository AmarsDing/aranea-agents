<template>
  <footer class="skill-pagination q-mt-md">
    <div class="text-caption text-grey-7">{{ total }} {{ label }}</div>
    <div class="row items-center q-gutter-sm">
      <q-select :model-value="pageSize" dense outlined emit-value map-options label="行" :options="pageSizeOptions" class="skill-pagination__page-size" @update:model-value="emit('update:pageSize', Number($event))" />
      <span class="text-caption">第 {{ page }} / {{ pageMax }} 页</span>
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
  display: flex
  align-items: center
  justify-content: space-between
  gap: 12px

.skill-pagination__page-size
  min-width: 96px

@media (max-width: 720px)
  .skill-pagination
    flex-direction: column
    align-items: stretch
</style>
