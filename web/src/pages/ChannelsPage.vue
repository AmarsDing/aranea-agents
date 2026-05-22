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
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import ChannelCatalogFilters from "../components/channels/ChannelCatalogFilters.vue";
import ChannelHeroSection from "../components/channels/ChannelHeroSection.vue";
import ChannelsTable from "../components/channels/ChannelsTable.vue";
import { channelConfig, channelMetadata, copyChannelWebhookURL } from "../components/channels/channelUi";
import ChannelEditorDialog from "../features/channels/ChannelEditorDialog.vue";
import { deleteChannel, listChannelCatalog, listChannelCredentials, listChannels, testChannel, toggleChannel } from "../features/channels/api";
import type { ChannelCatalogItem, ChannelCredential, ChannelRow } from "../features/channels/types";

const { t } = useI18n();
const $q = useQuasar();
const catalog = ref<ChannelCatalogItem[]>([]);
const rows = ref<ChannelRow[]>([]);
const loading = ref(false);
const error = ref("");
const search = ref("");
const typeFilter = ref("");
const statusFilter = ref("");
const togglingId = ref("");
const testingId = ref("");
const editorOpen = ref(false);
const editingRow = ref<ChannelRow | null>(null);
const editingCredentials = ref<ChannelCredential[]>([]);

const typeOptions = computed(() => catalog.value.map((item) => ({ label: item.label, value: item.type })));
const statusOptions = computed(() => [
  { label: t("channelsPage.enabled"), value: "enabled" },
  { label: t("channelsPage.disabled"), value: "disabled" },
  { label: t("channelsPage.statusActive"), value: "active" },
  { label: t("channelsPage.statusPendingAuth"), value: "pending_auth" },
  { label: t("channelsPage.statusError"), value: "error" }
]);

const filteredRows = computed(() => {
  const keyword = search.value.trim().toLowerCase();
  return rows.value.filter((row) => {
    const cfg = channelConfig(row);
    const meta = channelMetadata(row);
    if (typeFilter.value && cfg.type !== typeFilter.value) return false;
    if (statusFilter.value === "enabled" && !row.enabled) return false;
    if (statusFilter.value === "disabled" && row.enabled) return false;
    if (!["", "enabled", "disabled"].includes(statusFilter.value) && row.status !== statusFilter.value) return false;
    if (!keyword) return true;
    return [row.name, row.key, row.description, cfg.type, meta.external_id].some((value) => String(value || "").toLowerCase().includes(keyword));
  });
});

onMounted(loadAll);

function resetFilters() {
  search.value = "";
  typeFilter.value = "";
  statusFilter.value = "";
}

async function loadAll() {
  loading.value = true;
  error.value = "";
  try {
    const [catalogRows, channelRows] = await Promise.all([listChannelCatalog(), listChannels()]);
    catalog.value = catalogRows;
    rows.value = channelRows;
  } catch (err) {
    error.value = err instanceof Error ? err.message : t("channelsPage.loadFailed");
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editingRow.value = null;
  editingCredentials.value = [];
  editorOpen.value = true;
}

async function openEdit(row: ChannelRow) {
  editingRow.value = row;
  editingCredentials.value = await listChannelCredentials(row.id);
  editorOpen.value = true;
}

function onSaved(row: ChannelRow) {
  const index = rows.value.findIndex((item) => item.id === row.id);
  if (index >= 0) rows.value[index] = row;
  else rows.value.unshift(row);
}

async function toggleRow(row: ChannelRow, enabled: boolean) {
  togglingId.value = row.id;
  try {
    const updated = await toggleChannel(row.id, enabled);
    onSaved(updated);
    $q.notify({
      type: "positive",
      message: enabled ? t("channelsPage.toggleOkEnabled") : t("channelsPage.toggleOkDisabled")
    });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : t("channelsPage.toggleFailed") });
  } finally {
    togglingId.value = "";
  }
}

async function testRow(row: ChannelRow) {
  testingId.value = row.id;
  try {
    const result = await testChannel(row.id);
    $q.notify({ type: result.ok ? "positive" : "warning", message: result.message || result.status });
    await loadAll();
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : t("channelsPage.testFailed") });
  } finally {
    testingId.value = "";
  }
}

async function copyWebhook(row: ChannelRow) {
  try {
    const url = await copyChannelWebhookURL(row);
    $q.notify({ type: "positive", message: t("channelsPage.copyWebhookOk", { url }) });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : t("channelsPage.copyWebhookFailed") });
  }
}

function confirmDelete(row: ChannelRow) {
  $q.dialog({
    title: t("channelsPage.deleteTitle"),
    message: t("channelsPage.deleteMessage"),
    cancel: true,
    persistent: true
  }).onOk(async () => {
    await deleteChannel(row.id);
    rows.value = rows.value.filter((item) => item.id !== row.id);
    $q.notify({ type: "positive", message: t("channelsPage.deleteOk") });
  });
}
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
