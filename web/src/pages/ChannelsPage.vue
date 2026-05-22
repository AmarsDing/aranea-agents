<template>
  <q-page class="app-page-cream channels-page">
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
          class="channel-primary-btn"
          icon="add"
          :label="t('channelsPage.add')"
          @click="openCreate"
        />
        <q-btn
          outline
          rounded
          no-caps
          class="channel-outline-btn"
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

    <q-banner v-if="error" rounded class="channels-error-banner q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat dense :label="t('channelsPage.retry')" class="text-white" @click="loadAll" />
      </template>
    </q-banner>

    <ChannelsTable
      :rows="filteredRows"
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
import { useChannelsPage } from "../features/channels/useChannelsPage";

const {
  t,
  catalog,
  filteredRows,
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

<style scoped lang="sass">
.channels-page
  padding: 24px

.channels-error-banner
  background: rgba(229, 92, 92, 0.92)
  color: var(--color-on-accent)
  border: 1px solid rgba(255, 255, 255, 0.25)

body.body--dark .channels-error-banner
  background: rgba(255, 94, 122, 0.22)
  color: var(--color-text-primary)
  border-color: rgba(255, 255, 255, 0.12)

.channel-primary-btn
  background: var(--color-accent)
  color: var(--color-on-accent)

body:not(.body--dark) .channel-primary-btn:hover
  background: var(--color-accent-hover)

.channel-outline-btn
  border-color: rgba(208, 192, 168, 0.85)
  color: var(--color-text-primary)

body:not(.body--dark) .channel-outline-btn:hover
  background: var(--interaction-surface-hover)
</style>
