<template>
  <footer class="app-registry-pagination app-registry-pagination--card q-mt-md">
    <div class="app-registry-pagination__summary">{{ total }} {{ label }}</div>
    <div class="app-registry-pagination__controls row items-center no-wrap">
      <q-select
        :model-value="pageSize"
        dense
        outlined
        emit-value
        map-options
        label="每页"
        :options="pageSizeOptionsResolved"
        class="app-registry-pagination__page-size app-glass-control"
        @update:model-value="emit('update:pageSize', Number($event))"
      />
      <span class="app-registry-pagination__page-label">第 {{ page }} / {{ pageMax }} 页</span>
      <q-btn
        round
        dense
        flat
        icon="chevron_left"
        :disable="page <= 1 || loading"
        @click="emit('update:page', page - 1)"
      />
      <q-btn
        round
        dense
        flat
        icon="chevron_right"
        :disable="page >= pageMax || loading"
        @click="emit('update:page', page + 1)"
      />
    </div>
  </footer>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(
  defineProps<{
    page: number;
    pageSize: number;
    pageMax: number;
    total: number;
    loading?: boolean;
    label?: string;
    /** 默认 10 / 20 / 50，与 Skill 管理页一致 */
    pageSizeOptions?: number[];
  }>(),
  {
    loading: false,
    label: '条',
    pageSizeOptions: () => [10, 20, 50],
  },
);

const emit = defineEmits<{
  'update:page': [value: number];
  'update:pageSize': [value: number];
}>();

const pageSizeOptionsResolved = computed(() => props.pageSizeOptions.map((value) => ({ label: String(value), value })));
</script>
