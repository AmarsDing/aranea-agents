<template>
  <AppRegistryPagination
    :page="page"
    :page-size="pageSize"
    :page-max="pageMax"
    :total="total"
    :loading="loading"
    :label="label"
    @update:page="emit('update:page', $event)"
    @update:page-size="emit('update:pageSize', $event)"
  />
</template>

<script setup lang="ts">
import AppRegistryPagination from '../layout/AppRegistryPagination.vue';

defineProps<{
  page: number;
  pageSize: number;
  pageMax: number;
  total: number;
  loading?: boolean;
  label: string;
}>();

const emit = defineEmits<{
  'update:page': [value: number];
  'update:pageSize': [value: number];
}>();
</script>
