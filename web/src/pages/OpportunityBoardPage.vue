<template>
  <q-page padding class="q-gutter-md">
    <div class="text-h5">商机看板</div>
    <div class="text-caption text-grey-7">NovaTrade 情报 / 选品 50/50；Webhook 仅情报，不下单。</div>
    <div class="row q-gutter-sm">
      <q-btn-toggle v-model="audience" toggle-color="primary" :options="[{label:'全部',value:''},{label:'NovaTrade',value:'novatrade_intel'},{label:'选品',value:'commerce_sourcing'}]" @update:model-value="refresh" />
      <q-input v-model="title" dense outlined label="标题" class="col" />
      <q-btn color="primary" label="创建" @click="create" />
    </div>
    <q-list bordered separator>
      <q-item v-for="o in rows" :key="o.ID">
        <q-item-section>
          <q-item-label>{{ o.Title }} · {{ o.Audience }} · {{ o.Status }}</q-item-label>
          <q-item-label caption>{{ o.Summary }}</q-item-label>
        </q-item-section>
        <q-item-section side>
          <q-btn flat dense label="确认" @click="ack(o.ID)" />
        </q-item-section>
      </q-item>
    </q-list>
  </q-page>
</template>
<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { mvpApi, type Opportunity } from '../features/mvp/api';
const audience = ref('');
const title = ref('');
const rows = ref<Opportunity[]>([]);
async function refresh() {
  const res = await mvpApi.listOpp(audience.value);
  rows.value = res.items || [];
}
async function create() {
  await mvpApi.createOpp({ Title: title.value, Audience: audience.value || 'both', Summary: '', Score: 0.5, Status: 'open' });
  title.value = '';
  await refresh();
}
async function ack(id: string) { await mvpApi.ackOpp(id); await refresh(); }
onMounted(refresh);
</script>
