<template>
  <q-page padding class="q-gutter-md">
    <div class="text-h5">寻源看板</div>
    <div class="text-caption text-grey-7">MVP 深挖路径；approved_for_purchase 仅记账，无自动下单。</div>
    <div class="row q-gutter-sm items-end">
      <q-input v-model="name" dense outlined label="供应商" class="col" />
      <q-input v-model="url" dense outlined label="链接" class="col" />
      <q-input v-model="price" dense outlined label="报价区间" class="col" />
      <q-btn color="primary" label="添加" @click="create" />
    </div>
    <q-table flat bordered :rows="rows" :columns="cols" row-key="ID" hide-pagination :pagination="{ rowsPerPage: 0 }" />
  </q-page>
</template>
<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { mvpApi } from '../features/mvp/api';
const name = ref(''); const url = ref(''); const price = ref('');
const rows = ref<any[]>([]);
const cols = [
  { name: 'Name', label: '名称', field: 'Name', align: 'left' as const },
  { name: 'PriceRange', label: '报价', field: 'PriceRange' },
  { name: 'Status', label: '状态', field: 'Status' },
  { name: 'URL', label: '链接', field: 'URL' },
];
async function refresh() { rows.value = (await mvpApi.listSourcing()).items || []; }
async function create() {
  await mvpApi.createSupplier({ Name: name.value, URL: url.value, PriceRange: price.value, Status: 'suggested' });
  name.value = ''; url.value = ''; price.value = '';
  await refresh();
}
onMounted(refresh);
</script>
