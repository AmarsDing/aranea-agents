import { onMounted, ref } from "vue";
import { storeToRefs } from "pinia";
import { useQuasar } from "quasar";
import type { A2AInvokeResult } from "./types";
import { useA2AStore } from "../../stores/a2a";

export function useA2APage() {
  const $q = useQuasar();
  const a2aStore = useA2AStore();
  const { agentCards: agents, auditLog: auditRows, loading } = storeToRefs(a2aStore);

  const tab = ref("discover");
  const auditLoading = ref(false);
  const invokeLoading = ref(false);
  const error = ref("");
  const invokeResult = ref<A2AInvokeResult | null>(null);

  const discoverWorkspace = ref("");
  const discoverCapability = ref("");
  const invokeForm = ref({
    callee_agent_id: "",
    capability: "",
    payload_json: "{}",
    timeout_seconds: 30
  });

  const cardColumns = [
    { name: "agent_id", label: "Agent ID", field: "agent_id", align: "left" as const },
    { name: "display_name", label: "名称", field: "display_name", align: "left" as const },
    { name: "workspace", label: "Workspace", field: "workspace", align: "left" as const },
    { name: "enabled", label: "状态", field: "enabled", align: "left" as const },
    { name: "capabilities", label: "能力", field: "capabilities", align: "left" as const }
  ];

  const auditColumns = [
    { name: "created_at", label: "时间", field: "created_at", align: "left" as const },
    { name: "caller_agent_id", label: "调用方", field: "caller_agent_id", align: "left" as const },
    { name: "callee_agent_id", label: "被调方", field: "callee_agent_id", align: "left" as const },
    { name: "capability", label: "能力", field: "capability", align: "left" as const },
    { name: "status", label: "状态", field: "status", align: "left" as const },
    { name: "duration_ms", label: "耗时(ms)", field: "duration_ms", align: "right" as const }
  ];

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
        timeout_seconds: invokeForm.value.timeout_seconds || 30
      });
      const status = invokeResult.value.status;
      $q.notify({
        type: status === "success" ? "positive" : status === "error" ? "negative" : "info",
        message: `Invoke ${status}`
      });
      if (tab.value === "audit") await loadAudit();
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "Invoke 失败" });
    } finally {
      invokeLoading.value = false;
    }
  }

  function reload() {
    if (tab.value === "discover") void loadDiscover();
    else if (tab.value === "audit") void loadAudit();
  }

  onMounted(() => {
    void loadDiscover();
    void loadAudit();
  });

  return {
    agents,
    auditRows,
    loading,
    tab,
    auditLoading,
    invokeLoading,
    error,
    invokeResult,
    discoverWorkspace,
    discoverCapability,
    invokeForm,
    cardColumns,
    auditColumns,
    auditStatusColor,
    loadDiscover,
    loadAudit,
    submitInvoke,
    reload
  };
}
