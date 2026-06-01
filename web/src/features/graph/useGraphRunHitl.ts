import { ref, watch, type Ref } from "vue";
import { useQuasar } from "quasar";
import { useGraphStore } from "../../stores/graph";
import { buildResumePayload } from "./runtime/graphExecutionProjection";
import type { GraphInterruptInfo } from "./runtime/graphExecutionProjection";

export function useGraphRunHitl(
  execId: Ref<string>,
  interrupt: Ref<GraphInterruptInfo | null | undefined>,
  displayStatus: Ref<string>,
  clearInterrupt: () => void,
  refreshExecution: () => Promise<void>,
) {
  const $q = useQuasar();
  const graphStore = useGraphStore();

  const hitlDialogOpen = ref(false);
  const hitlAdvancedJson = ref("");
  const resumeLoading = ref(false);

  watch(interrupt, (value) => {
    if (value && displayStatus.value === "waiting_human") {
      hitlDialogOpen.value = true;
    }
  });

  function resumeExec() {
    hitlAdvancedJson.value = "";
    hitlDialogOpen.value = true;
  }

  async function submitHitlResume(approved: boolean) {
    if (!execId.value || !interrupt.value) return;
    resumeLoading.value = true;
    try {
      let advanced: Record<string, unknown> | undefined;
      if (hitlAdvancedJson.value.trim()) {
        try {
          advanced = JSON.parse(hitlAdvancedJson.value);
        } catch {
          throw new Error("恢复值 JSON 格式无效，请检查输入");
        }
      }
      const payload = buildResumePayload(interrupt.value, approved, advanced);
      await graphStore.resumeExecution(execId.value, payload);
      hitlDialogOpen.value = false;
      clearInterrupt();
      $q.notify({ type: "positive", message: approved ? "已恢复执行" : "已拒绝并恢复" });
      await refreshExecution();
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "恢复失败" });
    } finally {
      resumeLoading.value = false;
    }
  }

  return {
    hitlDialogOpen,
    hitlAdvancedJson,
    resumeLoading,
    resumeExec,
    submitHitlResume,
  };
}
