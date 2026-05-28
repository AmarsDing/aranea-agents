import { computed, reactive, ref, watch } from "vue";
import { useQuasar } from "quasar";
import type { McpUserCredential } from "./types";
import { useMcpStore } from "../../stores/mcp";

export function useMcpUserCredentialDialog(
  modelValue: () => boolean,
  mcpServerId: () => string,
  userId: () => string,
  emit: {
    (e: "update:modelValue", value: boolean): void;
    (e: "saved"): void;
  }
) {
  const $q = useQuasar();
  const mcpStore = useMcpStore();
  const loading = ref(false);
  const saving = ref(false);
  const items = ref<McpUserCredential[]>([]);
  const form = reactive({ credential_key: "Authorization", secret: "" });

  const canSave = computed(
    () => Boolean(mcpServerId() && userId() && form.credential_key.trim() && form.secret.trim())
  );

  watch(
    () => [modelValue(), mcpServerId(), userId()] as const,
    ([open]) => {
      if (open) void reload();
    }
  );

  async function reload() {
    const serverId = mcpServerId();
    const uid = userId();
    if (!serverId || !uid) return;
    loading.value = true;
    try {
      items.value = await mcpStore.fetchUserCredentials(serverId, uid);
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "加载凭据失败" });
    } finally {
      loading.value = false;
    }
  }

  async function save() {
    if (!canSave.value) return;
    saving.value = true;
    try {
      await mcpStore.saveUserCredential(mcpServerId(), userId(), {
        credential_key: form.credential_key.trim(),
        secret: form.secret.trim()
      });
      form.secret = "";
      await reload();
      emit("saved");
      $q.notify({ type: "positive", message: "凭据已保存" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "保存失败" });
    } finally {
      saving.value = false;
    }
  }

  function confirmRemove(credentialKey: string) {
    $q.dialog({
      title: "删除凭据",
      message: `确定删除凭据「${credentialKey}」？删除后 Agent 将无法使用该凭据访问 MCP 服务。`,
      cancel: true,
      persistent: true,
    }).onOk(() => void remove(credentialKey));
  }

  async function remove(credentialKey: string) {
    try {
      await mcpStore.removeUserCredential(mcpServerId(), userId(), credentialKey);
      await reload();
      $q.notify({ type: "positive", message: "已删除" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "删除失败" });
    }
  }

  return { loading, saving, items, form, canSave, reload, save, remove: confirmRemove };
}
