<template>
  <q-page padding class="q-gutter-md">
    <div class="text-h5">审核台</div>
    <div class="text-caption text-grey-7">单人终审；A=通过并打包，R=驳回（必填原因）。未过审零外发。Publish=Package A，非平台自动发。</div>
    <q-banner v-if="current" class="bg-grey-2">
      <div class="text-subtitle1">{{ current.DraftTitle }}</div>
      <div class="q-mt-sm" style="white-space: pre-wrap">{{ current.DraftBody }}</div>
      <div class="q-mt-sm text-negative" v-if="current.MachineFlags?.length">机审提示：{{ current.MachineFlags.join(', ') }}</div>
      <div class="q-mt-md row q-gutter-sm">
        <q-btn color="positive" label="通过并打包 (A)" @click="approve" />
        <q-input v-model="rejectNote" dense outlined label="驳回原因" class="col" />
        <q-btn color="negative" label="驳回 (R)" @click="reject" />
      </div>
    </q-banner>
    <div v-else class="text-grey">暂无待审</div>
    <q-list bordered v-if="lastJob">
      <q-item>
        <q-item-section>
          <q-item-label>最近发布包：{{ lastJob.Status }} / {{ lastJob.ID }}</q-item-label>
          <q-item-label caption>请人工粘贴到平台后台；不是「已发布到平台」</q-item-label>
        </q-item-section>
      </q-item>
    </q-list>
  </q-page>
</template>
<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue';
import { mvpApi, type ReviewTask, type PublishJob } from '../features/mvp/api';
const queue = ref<ReviewTask[]>([]);
const current = ref<ReviewTask | null>(null);
const rejectNote = ref('');
const lastJob = ref<PublishJob | null>(null);
async function refresh() {
  const res = await mvpApi.listReview();
  queue.value = res.items || [];
  current.value = queue.value[0] || null;
}
async function approve() {
  if (!current.value) return;
  const res = await mvpApi.approve(current.value.ID);
  lastJob.value = res.publish_job;
  await refresh();
}
async function reject() {
  if (!current.value) return;
  if (!rejectNote.value.trim()) { alert('驳回必填原因'); return; }
  await mvpApi.reject(current.value.ID, rejectNote.value);
  rejectNote.value = '';
  await refresh();
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'a' || e.key === 'A') void approve();
  if (e.key === 'r' || e.key === 'R') void reject();
  if (e.key === 'n' || e.key === 'N') {
    const i = queue.value.findIndex((x) => x.ID === current.value?.ID);
    current.value = queue.value[i + 1] || queue.value[0] || null;
  }
}
onMounted(() => { void refresh(); window.addEventListener('keydown', onKey); });
onUnmounted(() => window.removeEventListener('keydown', onKey));
</script>
