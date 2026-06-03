<template>
  <q-page class="app-standard-page app-registry-page webhooks-page">
    <AppPageHero
      :kicker="t('webhooksPage.kicker')"
      :title="t('webhooksPage.title')"
      :subtitle="t('webhooksPage.subtitle')"
    >
      <template #actions>
        <q-btn
          color="primary"
          rounded
          unelevated
          no-caps
          icon="add"
          :label="t('webhooksPage.btnCreate')"
          @click="openCreate"
        />
      </template>
    </AppPageHero>

    <AppPageToolbar>
      <q-input
        v-model="search"
        class="app-page-toolbar__search"
        dense
        outlined
        clearable
        debounce="200"
        :label="t('webhooksPage.search')"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <template #actions>
        <q-btn
          flat
          rounded
          no-caps
          icon="refresh"
          :label="t('webhooksPage.btnRefresh')"
          :loading="loading"
          @click="loadRows"
        />
      </template>
    </AppPageToolbar>

    <q-banner v-if="error" rounded class="app-page-error-banner q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat dense :label="t('webhooksPage.retry')" class="text-white" @click="loadRows" />
      </template>
    </q-banner>

    <div class="app-registry-table-shell">
      <WebhooksTable
        :rows="pagedRows"
        :loading="loading"
        :toggling-id="busyId"
        @toggle-enabled="toggleEnabled"
        @edit="openEdit"
        @remove="confirmDelete"
      />

      <AppRegistryPagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="filteredRows.length"
        :loading="loading"
        :label="t('webhooksPage.paginationLabel')"
      />
    </div>

    <WebhookDialog
      ref="dialogRef"
      v-model:open="editorOpen"
      :editing-id="editingId"
      :saving="saving"
      @save="saveWebhook"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import WebhooksTable from '../components/webhooks/WebhooksTable.vue';
import WebhookDialog from '../components/webhooks/WebhookDialog.vue';
import type { WebhookRow } from '../features/webhooks/types';
import { useWebhooksStore } from '../stores/webhooks';

const { t } = useI18n();
const $q = useQuasar();
const webhooksStore = useWebhooksStore();
const { webhooks: storeRows, loading: storeLoading } = storeToRefs(webhooksStore);
const loading = storeLoading;
const saving = ref(false);
const error = ref('');
const search = ref('');
const rows = storeRows;
const editorOpen = ref(false);
const editingId = ref('');
const busyId = ref('');
const dialogRef = ref<InstanceType<typeof WebhookDialog> | null>(null);

const filteredRows = computed(() => {
  const q = search.value.trim().toLowerCase();
  if (!q) return rows.value;
  return rows.value.filter((r) => r.name.toLowerCase().includes(q) || r.url.toLowerCase().includes(q));
});

const page = ref(1);
const pageSize = ref(20);
const pageMax = computed(() => Math.max(1, Math.ceil(filteredRows.value.length / pageSize.value)));
const pagedRows = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return filteredRows.value.slice(start, start + pageSize.value);
});

watch(search, () => {
  page.value = 1;
});

async function loadRows() {
  error.value = '';
  try {
    await webhooksStore.loadWebhooks();
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  }
}

function openCreate() {
  editingId.value = '';
  dialogRef.value?.reset(true);
  editorOpen.value = true;
}

function openEdit(row: WebhookRow) {
  editingId.value = row.id;
  dialogRef.value?.fill(row);
  editorOpen.value = true;
}

async function saveWebhook() {
  const payload = dialogRef.value?.getPayload();
  if (!payload || !payload.name?.trim() || !payload.url?.trim()) {
    $q.notify({ type: 'warning', message: t('webhooksPage.notifyRequired') });
    return;
  }
  saving.value = true;
  try {
    if (editingId.value) {
      await webhooksStore.saveWebhook(editingId.value, payload);
    } else {
      await webhooksStore.addWebhook(payload);
    }
    editorOpen.value = false;
    $q.notify({ type: 'positive', message: t('webhooksPage.notifySaved') });
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  } finally {
    saving.value = false;
  }
}

async function toggleEnabled(row: WebhookRow, enabled: boolean) {
  busyId.value = row.id;
  try {
    await webhooksStore.saveWebhook(row.id, { enabled });
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  } finally {
    busyId.value = '';
  }
}

function confirmDelete(row: WebhookRow) {
  $q.dialog({
    title: t('webhooksPage.confirmDeleteTitle'),
    message: t('webhooksPage.confirmDeleteMessage', { name: row.name }),
    cancel: true,
    persistent: true,
  }).onOk(async () => {
    try {
      await webhooksStore.removeWebhook(row.id);
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    }
  });
}

onMounted(loadRows);
</script>
