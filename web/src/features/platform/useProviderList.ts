import { computed, ref, watch, type ComputedRef, type Ref } from "vue";
import { useQuasar } from "quasar";
import type { PlatformResource, PlatformResourceName } from "./types";
import { errorMessage, getConfig, getCategories } from "./providerUtils";
import { usePlatformStore } from "../../stores/platform";
import { useProviderTrendDialog } from "../usage/useProviderTrendDialog";

export function useProviderList(deps: {
  resource: ComputedRef<PlatformResourceName>;
  isProviderResource: ComputedRef<boolean>;
  saving: Ref<boolean>;
}) {
  const platformStore = usePlatformStore();
  const $q = useQuasar();

  const rows = ref<PlatformResource[]>([]);
  const loading = ref(false);
  const keyword = ref("");
  const page = ref(1);
  const rowsPerPage = ref(20);
  const providerTypeFilter = ref<string[]>([]);

  const credentialEncryptionAvailable = computed(() => platformStore.credentialEncryptionAvailable);

  type ListKeyEntry = { visible: boolean; revealing: boolean; value: string };
  const listKeyById = ref<Record<string, ListKeyEntry>>({});

  function listKeyState(id: string): ListKeyEntry {
    return listKeyById.value[id] ?? { visible: false, revealing: false, value: "" };
  }

  function setListKeyState(id: string, patch: Partial<ListKeyEntry>) {
    const prev = listKeyState(id);
    listKeyById.value = { ...listKeyById.value, [id]: { ...prev, ...patch } };
  }

  async function toggleListKeyReveal(row: PlatformResource) {
    const id = row.id;
    const cur = listKeyState(id);
    if (cur.visible) {
      setListKeyState(id, { visible: false, revealing: false, value: "" });
      return;
    }
    setListKeyState(id, { revealing: true });
    try {
      const creds = await platformStore.revealCredentials(id);
      const cfg = getConfig(row);
      const plain = creds.api_key?.trim() || creds.secret_key?.trim() || "";
      if (!plain) {
        const cannotDecrypt =
          creds.has_api_key ||
          creds.has_secret_key ||
          cfg.api_key_set ||
          Boolean(cfg.secret_id?.trim());
        $q.notify({
          type: "warning",
          message: cannotDecrypt
            ? "密钥已保存，但无法解密显示。请在「系统设置」确认凭据加密密钥，或在编辑页重新保存 API Key。"
            : "未找到可显示的密钥，请在编辑页配置。"
        });
        return;
      }
      setListKeyState(id, { visible: true, revealing: false, value: plain });
    } catch (error) {
      setListKeyState(id, { revealing: false });
      $q.notify({ type: "negative", message: errorMessage(error) });
    }
  }

  const trendDialogOpen = ref(false);
  const trendRow = ref<PlatformResource | null>(null);
  const providerTrend = useProviderTrendDialog(trendDialogOpen, trendRow);

  const filteredRows = computed(() => {
    let list = rows.value;
    if (deps.isProviderResource.value && providerTypeFilter.value.length) {
      const allowed = new Set(providerTypeFilter.value.map((v) => v.toLowerCase()));
      list = list.filter((row) => allowed.has((getConfig(row).provider_type || "openai").toLowerCase()));
    }
    const q = keyword.value.trim().toLowerCase();
    if (!q) return list;
    return list.filter((row) =>
      [
        row.key,
        row.name,
        row.description,
        row.provider,
        row.model,
        row.agent_id,
        getConfig(row).provider_display_name,
        ...getCategories(row).map((category) => category.label)
      ].some((value) =>
        (value || "").toLowerCase().includes(q)
      )
    );
  });

  const pageCount = computed(() => Math.max(1, Math.ceil(filteredRows.value.length / rowsPerPage.value)));
  const pagedProviderRows = computed(() => {
    const start = (page.value - 1) * rowsPerPage.value;
    return filteredRows.value.slice(start, start + rowsPerPage.value);
  });

  watch(filteredRows, () => {
    if (page.value > pageCount.value) page.value = pageCount.value;
  });

  async function loadRows() {
    if (!deps.resource.value) return;
    loading.value = true;
    try {
      rows.value = await platformStore.loadResource(deps.resource.value);
      if (deps.isProviderResource.value && platformStore.credentialEncryptionAvailable === null) {
        void platformStore.loadCredentialStatus();
      }
    } catch (error) {
      $q.notify({ type: "negative", message: errorMessage(error) || "加载资源列表失败" });
    } finally {
      loading.value = false;
    }
  }

  async function toggleEnabled(row: PlatformResource, enabled: boolean) {
    deps.saving.value = true;
    try {
      const updated = await platformStore.editResource(deps.resource.value, row.id, { enabled });
      rows.value = rows.value.map((item) => (item.id === updated.id ? updated : item));
    } catch (error) {
      $q.notify({ type: "negative", message: errorMessage(error) || "切换启用状态失败" });
    } finally {
      deps.saving.value = false;
    }
  }

  function confirmRemoveRow(row: PlatformResource) {
    $q.dialog({
      title: "确认删除",
      message: `确定删除「${row.name}」吗？`,
      cancel: true,
      persistent: true
    }).onOk(() => {
      void removeRow(row);
    });
  }

  async function removeRow(row: PlatformResource) {
    try {
      await platformStore.removeResource(deps.resource.value, row.id);
      rows.value = rows.value.filter((item) => item.id !== row.id);
      $q.notify({ type: "positive", message: "已删除" });
    } catch (error) {
      $q.notify({ type: "negative", message: errorMessage(error) || "删除失败" });
    }
  }

  function openTrend(row: PlatformResource) {
    trendRow.value = row;
    trendDialogOpen.value = true;
  }

  return {
    rows,
    loading,
    keyword,
    page,
    rowsPerPage,
    providerTypeFilter,
    filteredRows,
    pageCount,
    pagedProviderRows,
    loadRows,
    toggleEnabled,
    confirmRemoveRow,
    openTrend,
    listKeyState,
    toggleListKeyReveal,
    trendDialogOpen,
    trendRow,
    providerTrend,
    credentialEncryptionAvailable,
  };
}
