import { onMounted, ref } from "vue";
import { storeToRefs } from "pinia";
import { useQuasar } from "quasar";
import type { A2AAgentCard, A2AInvokeResult, A2ARuntimeConfig, RegisterRemoteAgentInput, DiscoverRemoteInput } from "./types";
import { useA2AStore } from "../../stores/a2a";

import { getA2AConfig } from "./api";
import {
  A2A_AUDIT_TABLE_COLUMNS,
  A2A_CARD_TABLE_COLUMNS,
  A2A_REMOTE_TABLE_COLUMNS
} from "./a2aTableUi";

export function useA2APage() {
  const $q = useQuasar();
  const a2aStore = useA2AStore();
  const { agentCards: agents, auditLog: auditRows, remoteAgents, loading } = storeToRefs(a2aStore);

  const tab = ref("discover");
  const auditLoading = ref(false);
  const invokeLoading = ref(false);
  const remoteLoading = ref(false);
  const remoteDiscoverLoading = ref(false);
  const error = ref("");
  const invokeResult = ref<A2AInvokeResult | null>(null);
  const runtimeConfig = ref<A2ARuntimeConfig | null>(null);
  const remotePreview = ref<A2AAgentCard | null>(null);

  const discoverWorkspace = ref("");
  const discoverCapability = ref("");
  const remoteWorkspace = ref("");
  const invokeForm = ref({
    callee_agent_id: "",
    capability: "",
    payload_json: "{}",
    timeout_seconds: 30,
    workspace: ""
  });

  const cardColumns = A2A_CARD_TABLE_COLUMNS;
  const remoteColumns = A2A_REMOTE_TABLE_COLUMNS;
  const auditColumns = A2A_AUDIT_TABLE_COLUMNS;

  function auditStatusColor(status: string) {
    if (status === "success") return "positive";
    if (status === "error" || status === "timeout") return "negative";
    return "grey";
  }

  async function loadDiscover() {
    error.value = "";
    try {
      await a2aStore.discover({
        workspace: discoverWorkspace.value.trim(),
        capability: discoverCapability.value.trim()
      });
    } catch (e) {
      error.value = e instanceof Error ? e.message : "发现失败";
    }
  }

  async function loadRemote() {
    remoteLoading.value = true;
    error.value = "";
    try {
      await a2aStore.loadRemoteAgents(remoteWorkspace.value.trim());
    } catch (e) {
      error.value = e instanceof Error ? e.message : "加载远程注册失败";
    } finally {
      remoteLoading.value = false;
    }
  }

  async function loadAudit() {
    auditLoading.value = true;
    error.value = "";
    try {
      await a2aStore.loadAudit({ limit: 100 });
    } catch (e) {
      error.value = e instanceof Error ? e.message : "加载审计失败";
    } finally {
      auditLoading.value = false;
    }
  }

  async function submitInvoke() {
    if (!invokeForm.value.callee_agent_id.trim() || !invokeForm.value.capability.trim()) {
      $q.notify({ type: "warning", message: "请填写 Agent ID 与 Capability" });
      return;
    }
    invokeLoading.value = true;
    invokeResult.value = null;
    try {
      invokeResult.value = await a2aStore.invoke({
        callee_agent_id: invokeForm.value.callee_agent_id.trim(),
        capability: invokeForm.value.capability.trim(),
        payload_json: invokeForm.value.payload_json.trim() || "{}",
        timeout_seconds: invokeForm.value.timeout_seconds || 30,
        workspace: invokeForm.value.workspace.trim()
      });
      const status = invokeResult.value.status;
      $q.notify({
        type: status === "success" ? "positive" : status === "error" ? "negative" : "info",
        message: `Invoke ${status}`
      });
      if (tab.value === "audit") await loadAudit();
      else await loadDiscover();
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "Invoke 失败" });
    } finally {
      invokeLoading.value = false;
    }
  }

  async function submitRemoteRegister(input: RegisterRemoteAgentInput) {
    if (!input.remote_url?.trim()) {
      $q.notify({ type: "warning", message: "请填写远程 URL" });
      return;
    }
    remoteLoading.value = true;
    try {
      await a2aStore.registerRemote(input);
      remotePreview.value = null;
      $q.notify({ type: "positive", message: "远程 Agent 已注册" });
      await loadRemote();
      await loadDiscover();
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "注册失败" });
    } finally {
      remoteLoading.value = false;
    }
  }

  async function previewRemote(input: DiscoverRemoteInput) {
    if (!input.remote_url?.trim()) {
      $q.notify({ type: "warning", message: "请填写远程 URL" });
      return;
    }
    remoteDiscoverLoading.value = true;
    remotePreview.value = null;
    try {
      remotePreview.value = await a2aStore.previewRemote(input);
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "Discover 失败" });
    } finally {
      remoteDiscoverLoading.value = false;
    }
  }

  async function removeRemote(id: string) {
    remoteLoading.value = true;
    try {
      await a2aStore.removeRemote(id);
      $q.notify({ type: "positive", message: "已删除" });
      await loadRemote();
      await loadDiscover();
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "删除失败" });
    } finally {
      remoteLoading.value = false;
    }
  }

  function reload() {
    if (tab.value === "discover") void loadDiscover();
    else if (tab.value === "audit") void loadAudit();
    else if (tab.value === "remote") void loadRemote();
  }

  onMounted(() => {
    void loadDiscover();
    void loadAudit();
    void loadRemote();
    getA2AConfig().then((c) => { runtimeConfig.value = c; }).catch(() => {});
  });

  return {
    agents,
    auditRows,
    remoteAgents,
    loading,
    tab,
    auditLoading,
    invokeLoading,
    remoteLoading,
    remoteDiscoverLoading,
    error,
    invokeResult,
    remotePreview,
    discoverWorkspace,
    discoverCapability,
    remoteWorkspace,
    invokeForm,
    cardColumns,
    remoteColumns,
    auditColumns,
    auditStatusColor,
    loadDiscover,
    loadRemote,
    loadAudit,
    submitInvoke,
    submitRemoteRegister,
    previewRemote,
    removeRemote,
    reload,
    runtimeConfig
  };
}
