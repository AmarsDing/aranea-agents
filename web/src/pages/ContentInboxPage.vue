<template>
  <q-page padding class="q-gutter-md">
    <div class="text-h5">内容候选库</div>
    <div class="text-caption text-grey-7">RSS / 搜索 / 手工入库（MVP）。过滤走现有 list API 的 source_type / status。</div>
    <q-card flat bordered>
      <q-card-section class="row q-gutter-sm items-end">
        <q-input v-model="title" label="标题" dense outlined class="col" />
        <q-input v-model="body" label="正文" dense outlined class="col" />
        <q-input v-model="url" label="URL" dense outlined class="col" />
        <q-btn color="primary" label="入库" @click="create" />
      </q-card-section>
      <q-card-section class="row q-gutter-sm items-end">
        <q-input v-model="feedURL" label="RSS Feed URL" dense outlined class="col" />
        <q-btn color="secondary" label="拉取 RSS" @click="pullRSS" />
        <q-input v-model="searchQuery" label="搜索词（手工 hits）" dense outlined class="col" />
        <q-input v-model="searchHitURL" label="Hit URL" dense outlined class="col" />
        <q-btn color="secondary" outline label="搜索入库" @click="pullSearch" />
      </q-card-section>
      <q-card-section class="row q-gutter-sm items-center">
        <q-select v-model="sourceType" dense outlined emit-value map-options label="来源" :options="sourceOpts" style="min-width:140px" @update:model-value="refresh" />
        <q-select v-model="status" dense outlined emit-value map-options label="状态" :options="statusOpts" style="min-width:140px" @update:model-value="refresh" />
        <q-btn flat icon="refresh" @click="refresh" />
        <div class="text-caption text-grey-7" v-if="ingestMsg">{{ ingestMsg }}</div>
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
const feedURL = ref('');
const searchQuery = ref('');
const searchHitURL = ref('');
const sourceType = ref('');
const status = ref('');
const ingestMsg = ref('');
const rows = ref<any[]>([]);
const sourceOpts = [
  { label: '全部来源', value: '' },
  { label: 'RSS', value: 'rss' },
  { label: '搜索', value: 'search' },
  { label: '手工', value: 'manual' },
];
const statusOpts = [
  { label: '全部状态', value: '' },
  { label: '候选', value: 'candidate' },
  { label: '已选题', value: 'selected' },
  { label: '审核中', value: 'in_review' },
  { label: '已通过', value: 'approved' },
  { label: '已驳回', value: 'rejected' },
];
const cols = [
  { name: 'Title', label: '标题', field: 'Title', align: 'left' as const },
  { name: 'SourceType', label: '来源', field: 'SourceType' },
  { name: 'Status', label: '状态', field: 'Status' },
];
async function refresh() {
  const qs = new URLSearchParams();
  if (sourceType.value) qs.set('source_type', sourceType.value);
  if (status.value) qs.set('status', status.value);
  const q = qs.toString() ? `?${qs.toString()}` : '';
  const res = await mvpApi.listContent(q);
  rows.value = res.items || [];
}
async function create() {
  await mvpApi.createContent({ Title: title.value, Body: body.value, SourceURL: url.value, SourceType: 'manual' });
  title.value = ''; body.value = ''; url.value = '';
  await refresh();
}
async function pullRSS() {
  if (!feedURL.value.trim()) { alert('请填写 Feed URL'); return; }
  const res = await mvpApi.ingestRSS(feedURL.value.trim());
  ingestMsg.value = `RSS: fetched=${res.fetched} created=${res.created} deduped=${res.deduped}`;
  await refresh();
}
async function pullSearch() {
  if (!searchQuery.value.trim() || !searchHitURL.value.trim()) { alert('请填写搜索词与至少一条 Hit URL'); return; }
  const res = await mvpApi.ingestSearch(searchQuery.value.trim(), [{
    URL: searchHitURL.value.trim(),
    Title: searchQuery.value.trim(),
    Snippet: body.value || searchQuery.value.trim(),
  }]);
  ingestMsg.value = `Search: fetched=${res.fetched} created=${res.created} deduped=${res.deduped}`;
  await refresh();
}
onMounted(refresh);
</script>
