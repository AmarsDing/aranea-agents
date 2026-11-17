<template>
  <q-page class="app-standard-page app-registry-page">
    <AppPageHero :title="t('uiSystem.operations.reviewTitle')" :subtitle="t('uiSystem.operations.reviewSubtitle')">
      <template #actions
        ><q-btn
          outline
          color="primary"
          icon="refresh"
          :label="t('uiSystem.operations.refresh')"
          :loading="loading"
          :disable="saving"
          @click="store.load"
      /></template>
    </AppPageHero>
    <!-- 五态：已有待审数据但刷新失败 → 局部失败横幅，队列照常渲染 -->
    <AppStatePanel
      v-if="(error || forbidden) && rows.length > 0"
      variant="partial"
      dense
      class="q-mb-md"
      :description="error"
      :action-label="t('uiSystem.operations.retry')"
      action-icon="refresh"
      @action="store.load"
    />
    <!-- 五态：无权限 → 只渲染 forbidden 面板，不提供重试 -->
    <AppStatePanel v-else-if="forbidden" variant="forbidden" class="q-mb-md" />
    <!-- 五态：加载失败且无待审数据 → 只渲染可重试错误面板，不与空态并存。 -->
    <AppStatePanel
      v-else-if="error"
      variant="error"
      class="q-mb-md"
      :description="error"
      :action-label="t('uiSystem.operations.retry')"
      action-icon="refresh"
      @action="store.load"
    />
    <q-card v-if="current" flat class="app-registry-panel">
      <q-card-section
        ><div class="ui-review-meta">
          <q-chip color="warning" icon="schedule">{{
            t('uiSystem.operations.reviewCount', { count: rows.length })
          }}</q-chip
          ><span>{{ t('uiSystem.operations.shortcutHint') }}</span>
        </div>
        <h2 class="ui-card-title q-mt-md">{{ current.DraftTitle }}</h2>
        <div class="ui-review-body q-mt-md">{{ current.DraftBody }}</div></q-card-section
      >
      <q-card-section v-if="current.MachineFlags?.length"
        ><q-banner class="bg-warning"
          ><template #avatar><q-icon name="gpp_maybe" /></template
          >{{ t('uiSystem.operations.flags', { flags: current.MachineFlags.join(', ') }) }}</q-banner
        ></q-card-section
      >
      <q-separator />
      <q-card-section
        ><q-input
          ref="rejectInput"
          v-model="rejectNote"
          type="textarea"
          autogrow
          dense
          outlined
          :label="t('uiSystem.operations.rejectReason')"
          :disable="saving" />
        <div class="app-actions-bar q-mt-md">
          <q-btn
            flat
            :label="t('uiSystem.operations.nextItem')"
            :disable="saving || loading || rows.length < 2"
            @click="next"
          /><q-btn
            outline
            color="negative"
            :label="t('uiSystem.operations.reject')"
            :disable="saving || loading || !rejectNote.trim()"
            @click="requestReject"
          /><q-btn
            color="primary"
            unelevated
            :label="t('uiSystem.operations.approve')"
            :loading="saving"
            :disable="saving || loading"
            @click="requestApprove"
          /></div
      ></q-card-section>
    </q-card>
    <!-- 五态：首屏加载无待审数据 → 骨架面板 -->
    <AppStatePanel v-else-if="loading && !error" variant="loading" :rows="4" />
    <!-- 五态：成功但无待审数据 → 空态 -->
    <AppStatePanel
      v-else-if="!error"
      variant="empty"
      :title="t('uiSystem.operations.noReview')"
      :description="t('uiSystem.operations.reviewEmpty')"
      icon="fact_check"
    />
    <q-card v-if="lastJob" flat class="q-mt-lg"
      ><q-card-section
        ><h2 class="ui-card-title">{{ t('uiSystem.operations.packageTitle') }}</h2>
        <p class="ui-card-description q-mt-sm">
          {{ lastJob.Status }} / {{ lastJob.ID }} · {{ t('uiSystem.operations.packageNote') }}
        </p>
        <div class="ui-preview-actions q-mt-md">
          <q-btn
            outline
            color="primary"
            :label="t('uiSystem.operations.manifest')"
            :href="manifestURL"
            target="_blank"
            rel="noopener noreferrer"
          /><q-btn
            outline
            color="primary"
            :label="t('uiSystem.operations.bodyFile')"
            :href="bodyURL"
            target="_blank"
            rel="noopener noreferrer"
          /></div></q-card-section
    ></q-card>
  </q-page>
</template>
<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';
import type { QInput } from 'quasar';
import { confirm } from '../composables/useAppConfirm';
import { useReviewQueueStore } from '../stores/mvp';
import { packageFileURL } from '../features/mvp/links';
import { reviewShortcut } from '../features/mvp/reviewShortcuts';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
const { t } = useI18n();
const store = useReviewQueueStore();
const { rows, lastJob, loading, saving, error, forbidden } = storeToRefs(store);
const currentId = ref('');
const rejectNote = ref('');
const rejectInput = ref<QInput>();
const current = computed(() => rows.value.find((item) => item.ID === currentId.value) ?? rows.value[0] ?? null);
watch(
  () => current.value?.ID,
  () => {
    rejectNote.value = '';
  },
);
const manifestURL = computed(() => (lastJob.value ? packageFileURL(lastJob.value.ID, 'manifest.json') : '#'));
const bodyURL = computed(() => (lastJob.value ? packageFileURL(lastJob.value.ID, 'body.md') : '#'));
function next() {
  if (saving.value || loading.value || !rows.value.length) return;
  const index = rows.value.findIndex((item) => item.ID === current.value?.ID);
  currentId.value = rows.value[(index + 1) % rows.value.length].ID;
  rejectNote.value = '';
}
async function requestApprove() {
  if (!current.value || saving.value || loading.value) return;
  const item = current.value;
  const ok = await confirm({
    title: t('uiSystem.operations.approveTitle'),
    message: t('uiSystem.operations.approveMessage', { title: item.DraftTitle }),
    tone: 'primary',
  });
  if (!ok) return;
  if (current.value?.ID === item.ID && !saving.value && !loading.value) void store.approve(item.ID);
}
async function requestReject() {
  if (!current.value || saving.value || loading.value) return;
  if (!rejectNote.value.trim()) {
    rejectInput.value?.focus();
    return;
  }
  const id = current.value.ID;
  const note = rejectNote.value.trim();
  const ok = await confirm({
    title: t('uiSystem.operations.rejectTitle'),
    message: note,
    tone: 'danger',
  });
  if (!ok) return;
  if (current.value?.ID === id && !saving.value && !loading.value && (await store.reject(id, note)))
    rejectNote.value = '';
}
function onKey(event: KeyboardEvent) {
  if (loading.value || saving.value || !current.value) return;
  const action = reviewShortcut(event);
  if (!action) return;
  event.preventDefault();
  if (action === 'approve') requestApprove();
  else if (action === 'reject') requestReject();
  else next();
}
onMounted(() => {
  void store.load();
  window.addEventListener('keydown', onKey);
});
onUnmounted(() => window.removeEventListener('keydown', onKey));
</script>
