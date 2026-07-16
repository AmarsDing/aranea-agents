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
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import WebhooksTable from '../components/webhooks/WebhooksTable.vue';
import WebhookDialog from '../components/webhooks/WebhookDialog.vue';
import { useWebhooksPage } from '../features/webhooks/useWebhooksPage';

const {
  t,
  loading,
  saving,
  error,
  search,
  editorOpen,
  editingId,
  busyId,
  dialogRef,
  page,
  pageSize,
  total,
  filteredRows,
  pagedRows,
  pageMax,
  loadRows,
  openCreate,
  openEdit,
  saveWebhook,
  toggleEnabled,
  confirmDelete,
} = useWebhooksPage();
</script>
