<template>
  <q-page class="app-standard-page app-registry-page channels-page">
    <ChannelHeroSection
      :kicker="t('channelsPage.kicker')"
      :title="t('channelsPage.title')"
      :subtitle="t('channelsPage.subtitle')"
    >
      <template #actions>
        <q-btn
          rounded
          no-caps
          unelevated
          class="app-accent-btn"
          icon="add"
          :label="t('channelsPage.add')"
          @click="openCreate"
        />
        <q-btn
          outline
          rounded
          no-caps
          class="app-outline-btn"
          icon="refresh"
          :label="t('channelsPage.refresh')"
          :loading="loading"
          @click="loadAll"
        />
      </template>
    </ChannelHeroSection>

    <ChannelCatalogFilters
      :search="search"
      :type-filter="typeFilter"
      :status-filter="statusFilter"
      :type-options="typeOptions"
      :status-options="statusOptions"
      :loading="loading"
      @update:search="search = $event"
      @update:type-filter="typeFilter = $event"
      @update:status-filter="statusFilter = $event"
      @reset="resetFilters"
      @refresh="loadAll"
    />

    <q-banner v-if="error" rounded class="app-page-error-banner q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat dense :label="t('channelsPage.retry')" class="text-white" @click="loadAll" />
      </template>
    </q-banner>

    <ChannelsTable
      :rows="pagedRows"
      :catalog="catalog"
      :loading="loading"
      :toggling-id="togglingId"
      :testing-id="testingId"
      @toggle-enabled="toggleRow"
      @test-connection="testRow"
      @copy-webhook="copyWebhook"
      @edit="openEdit"
      @remove="confirmDelete"
    />

    <AppRegistryPagination
      v-model:page="page"
      v-model:page-size="pageSize"
      :page-max="pageMax"
      :total="filteredRows.length"
      :loading="loading"
      label="个 Channel"
    />

    <ChannelEditorDialog
      v-model="editorOpen"
      :catalog="catalog"
      :row="editingRow"
      :credentials="editingCredentials"
      @saved="onSaved"
      @tested="loadAll"
    />
  </q-page>
</template>

<script setup lang="ts">
import ChannelCatalogFilters from "../components/channels/ChannelCatalogFilters.vue";
import ChannelHeroSection from "../components/channels/ChannelHeroSection.vue";
import ChannelsTable from "../components/channels/ChannelsTable.vue";
import ChannelEditorDialog from "../features/channels/ChannelEditorDialog.vue";
import AppRegistryPagination from "../components/layout/AppRegistryPagination.vue";
import { useChannelsPage } from "../features/channels/useChannelsPage";

const {
  t,
  catalog,
  filteredRows,
  pagedRows,
  page,
  pageSize,
  pageMax,
  loading,
  error,
  search,
  typeFilter,
  statusFilter,
  typeOptions,
  statusOptions,
  togglingId,
  testingId,
  editorOpen,
  editingRow,
  editingCredentials,
  resetFilters,
  loadAll,
  openCreate,
  openEdit,
  onSaved,
  toggleRow,
  testRow,
  copyWebhook,
  confirmDelete
} = useChannelsPage();
</script>
