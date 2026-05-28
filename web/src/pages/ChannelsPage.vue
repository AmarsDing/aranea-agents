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
      @ops="openOps"
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

    <section v-if="opsChannel" ref="opsSectionRef" class="channel-ops-section">
      <div class="row items-center justify-between q-mb-sm">
        <div>
          <div class="text-subtitle1 text-weight-medium">{{ t("channelsPage.opsTitle") }}</div>
          <div class="text-caption text-grey-7">
            {{ opsChannel.name }} · {{ opsChannel.key }}
          </div>
        </div>
        <q-btn
          flat
          dense
          no-caps
          icon="close"
          :label="t('channelsPage.opsClose')"
          @click="closeOps"
        />
      </div>
      <div class="channel-ops-grid">
        <ChannelTurnJobsPanel :channel-id="opsChannel.id" />
        <ChannelDeliveriesPanel :channel-id="opsChannel.id" />
      </div>
    </section>

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
import { nextTick, ref, watch } from "vue";
import ChannelCatalogFilters from "../components/channels/ChannelCatalogFilters.vue";
import ChannelHeroSection from "../components/channels/ChannelHeroSection.vue";
import ChannelsTable from "../components/channels/ChannelsTable.vue";
import ChannelEditorDialog from "../features/channels/ChannelEditorDialog.vue";
import ChannelDeliveriesPanel from "../features/channels/ChannelDeliveriesPanel.vue";
import ChannelTurnJobsPanel from "../features/channels/ChannelTurnJobsPanel.vue";
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
  opsChannel,
  resetFilters,
  loadAll,
  openCreate,
  openEdit,
  openOps,
  closeOps,
  onSaved,
  toggleRow,
  testRow,
  copyWebhook,
  confirmDelete
} = useChannelsPage();

const opsSectionRef = ref<HTMLElement | null>(null);

watch(opsChannel, async (row) => {
  if (!row) return;
  await nextTick();
  opsSectionRef.value?.scrollIntoView({ behavior: "smooth", block: "start" });
});
</script>

<style scoped>
.channel-ops-section {
  margin-top: 16px;
  padding: 16px;
  border: 1px solid var(--glass-border);
  border-radius: 20px;
  background: var(--glass-elevated);
}

.channel-ops-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

@media (width <= 1100px) {
  .channel-ops-grid {
    grid-template-columns: 1fr;
  }
}
</style>
