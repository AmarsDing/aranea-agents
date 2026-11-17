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

    <AppStatePanel
      v-if="pageState === 'loading'"
      variant="loading"
      :rows="4"
    />
    <AppStatePanel
      v-else-if="pageState === 'error'"
      variant="error"
      :title="error"
      :action-label="t('webhooksPage.retry')"
      action-icon="refresh"
      @action="loadRows"
    />
    <AppStatePanel
      v-else-if="pageState === 'forbidden'"
      variant="forbidden"
    />
    <AppStatePanel
      v-else-if="pageState === 'empty'"
      variant="empty"
    />

    <div v-else class="app-registry-table-shell">
      <AppStatePanel
        v-if="pageState === 'partial'"
        class="q-mb-md"
        variant="partial"
        dense
        :description="error"
        :action-label="t('webhooksPage.retry')"
        action-icon="refresh"
        @action="loadRows"
      />
      <WebhooksTable
        :rows="pagedRows"
        :loading="loading"
        :toggling-id="busyId"
        :testing-id="testingId"
        @toggle-enabled="toggleEnabled"
        @edit="openEdit"
        @remove="confirmDelete"
        @test="testRow"
      />

      <AppRegistryPagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="total"
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
import { computed } from 'vue';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import WebhooksTable from '../components/webhooks/WebhooksTable.vue';
import WebhookDialog from '../components/webhooks/WebhookDialog.vue';
import { useWebhooksPage } from '../features/webhooks/useWebhooksPage';
import { resolvePageState } from './pageState';

const {
  t,
  loading,
  saving,
  error,
  forbidden,
  search,
  editorOpen,
  editingId,
  busyId,
  testingId,
  dialogRef,
  page,
  pageSize,
  total,
  pagedRows,
  pageMax,
  loadRows,
  openCreate,
  openEdit,
  saveWebhook,
  toggleEnabled,
  testRow,
  confirmDelete,
} = useWebhooksPage();

const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: pagedRows.value.length,
    forbidden: forbidden.value,
  }),
);
</script>
