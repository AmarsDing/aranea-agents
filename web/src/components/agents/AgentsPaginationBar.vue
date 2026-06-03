<template>
  <footer class="app-registry-pagination app-registry-pagination--card q-mt-md">
    <div class="app-registry-pagination__summary">{{ total }} 条</div>
    <div class="app-registry-pagination__controls row items-center no-wrap">
      <q-select
        v-model="rowsPerPage"
        dense
        outlined
        emit-value
        map-options
        label="行"
        :options="[10, 20, 50].map((v) => ({ label: String(v), value: v }))"
        class="app-registry-pagination__page-size app-glass-control"
      />
      <span class="app-registry-pagination__page-label">第 {{ page }} / {{ pageMax }} 页</span>
      <q-btn round dense flat icon="chevron_left" :disable="page <= 1" @click="decrementPage" />
      <q-btn round dense flat icon="chevron_right" :disable="page >= pageMax" @click="incrementPage" />
    </div>
  </footer>
</template>

<script setup lang="ts">
const page = defineModel<number>('page', { required: true });
const rowsPerPage = defineModel<number>('rowsPerPage', { required: true });

const props = defineProps<{
  total: number;
  pageMax: number;
}>();

function decrementPage() {
  page.value = Math.max(1, page.value - 1);
}

function incrementPage() {
  page.value = Math.min(props.pageMax, page.value + 1);
}
</script>
