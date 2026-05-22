import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import { storeToRefs } from "pinia";
import { channelConfig, channelMetadata, copyChannelWebhookURL } from "../../components/channels/channelUi";
import { useChannelsStore } from "../../stores/channels";
import type { ChannelCredential, ChannelRow } from "./types";

export function useChannelsPage() {
  const { t } = useI18n();
  const $q = useQuasar();
  const channelsStore = useChannelsStore();
  const { channels: rows, catalog, loading } = storeToRefs(channelsStore);

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
      return [row.name, row.key, row.description, cfg.type, meta.external_id].some((value) =>
        String(value || "").toLowerCase().includes(keyword)
      );
    });
  });

  onMounted(() => void loadAll());

  function resetFilters() {
    search.value = "";
    typeFilter.value = "";
    statusFilter.value = "";
  }

  async function loadAll() {
    error.value = "";
    try {
      await Promise.all([channelsStore.loadCatalog(), channelsStore.loadChannels()]);
    } catch (err) {
      error.value = err instanceof Error ? err.message : t("channelsPage.loadFailed");
    }
  }

  function openCreate() {
    editingRow.value = null;
    editingCredentials.value = [];
    editorOpen.value = true;
  }

  async function openEdit(row: ChannelRow) {
    editingRow.value = row;
    editingCredentials.value = await channelsStore.fetchCredentials(row.id);
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
      const updated = await channelsStore.toggle(row.id, enabled);
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
      const result = await channelsStore.testConnection(row.id);
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
      await channelsStore.removeChannel(row.id);
      $q.notify({ type: "positive", message: t("channelsPage.deleteOk") });
    });
  }

  return {
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
  };
}
