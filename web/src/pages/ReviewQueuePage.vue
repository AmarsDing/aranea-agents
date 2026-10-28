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
    <q-banner v-if="error" class="bg-negative q-mb-md" role="alert"
      >{{ error
      }}<template #action
        ><q-btn
          flat
          :label="t('uiSystem.operations.retry')"
          :disable="loading || saving"
          @click="store.load" /></template
    ></q-banner>
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
    <q-card v-else flat class="app-entity-empty q-pa-lg"
      ><q-spinner v-if="loading" color="primary" />
      <div v-else class="ui-card-description">
        {{ t('uiSystem.operations.noReview')
        }}{{ error ? t('uiSystem.operations.reviewRetry') : t('uiSystem.operations.reviewEmpty') }}
      </div></q-card
    >
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
import { useQuasar, type QInput } from 'quasar';
import { useReviewQueueStore } from '../stores/mvp';
import { packageFileURL } from '../features/mvp/links';
import { reviewShortcut } from '../features/mvp/reviewShortcuts';
import AppPageHero from '../components/layout/AppPageHero.vue';
const { t } = useI18n();
const store = useReviewQueueStore();
const { rows, lastJob, loading, saving, error } = storeToRefs(store);
const $q = useQuasar();
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
function requestApprove() {
  if (!current.value || saving.value || loading.value) return;
  const item = current.value;
  $q.dialog({
    title: t('uiSystem.operations.approveTitle'),
    message: t('uiSystem.operations.approveMessage', { title: item.DraftTitle }),
    cancel: true,
    persistent: true,
  }).onOk(() => {
    if (current.value?.ID === item.ID && !saving.value && !loading.value) void store.approve(item.ID);
  });
}
function requestReject() {
  if (!current.value || saving.value || loading.value) return;
  if (!rejectNote.value.trim()) {
    rejectInput.value?.focus();
    return;
  }
  const id = current.value.ID;
  const note = rejectNote.value.trim();
  $q.dialog({ title: t('uiSystem.operations.rejectTitle'), message: note, cancel: true, persistent: true }).onOk(
    async () => {
      if (current.value?.ID === id && !saving.value && !loading.value && (await store.reject(id, note)))
        rejectNote.value = '';
    },
  );
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
