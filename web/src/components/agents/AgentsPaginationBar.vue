<template>
  <footer class="agents-pagination q-mt-md">
    <div class="text-caption text-grey-7">{{ total }} 条</div>
    <div class="row items-center q-gutter-sm">
      <q-select
        v-model="rowsPerPage"
        dense
        outlined
        emit-value
        map-options
        label="行"
        :options="[10, 20, 50].map((v) => ({ label: String(v), value: v }))"
        class="rows-select"
      />
      <span class="text-caption">第 {{ page }} / {{ pageMax }} 页</span>
      <q-btn round dense flat icon="chevron_left" :disable="page <= 1" @click="decrementPage" />
      <q-btn round dense flat icon="chevron_right" :disable="page >= pageMax" @click="incrementPage" />
    </div>
  </footer>
</template>

<script setup lang="ts">
const page = defineModel<number>("page", { required: true });
const rowsPerPage = defineModel<number>("rowsPerPage", { required: true });

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

<style scoped>
.agents-pagination {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  padding: 12px 16px;
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.78);
}

.rows-select {
  width: 96px;
}
</style>
