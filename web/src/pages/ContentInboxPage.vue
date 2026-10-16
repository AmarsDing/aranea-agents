<template>
  <q-page padding class="q-gutter-md">
    <div class="text-h5">内容候选库</div>
    <div class="text-caption text-grey-7">RSS / 搜索 / 手工入库（MVP）</div>
    <q-card flat bordered>
      <q-card-section class="row q-gutter-sm items-end">
        <q-input v-model="title" label="标题" dense outlined class="col" />
        <q-input v-model="body" label="正文" dense outlined class="col" />
        <q-input v-model="url" label="URL" dense outlined class="col" />
        <q-btn color="primary" label="入库" @click="create" />
      </q-card-section>
    </q-card>
    <q-table flat bordered :rows="rows" :columns="cols" row-key="ID" hide-pagination :pagination="{ rowsPerPage: 0 }" />
  </q-page>
</template>
<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { mvpApi } from '../features/mvp/api';
const title = ref('');
const body = ref('');
const url = ref('');
const rows = ref<any[]>([]);
const cols = [
  { name: 'Title', label: '标题', field: 'Title', align: 'left' as const },
  { name: 'SourceType', label: '来源', field: 'SourceType' },
  { name: 'Status', label: '状态', field: 'Status' },
];
async function refresh() {
  const res = await mvpApi.listContent();
  rows.value = res.items || [];
}
async function create() {
  await mvpApi.createContent({ Title: title.value, Body: body.value, SourceURL: url.value, SourceType: 'manual' });
  title.value = ''; body.value = ''; url.value = '';
  await refresh();
}
onMounted(refresh);
</script>
