import { ref, type Ref } from "vue";
import { useChatRuntimeStore } from "../../../stores/chat/runtimeStore";
import {
  AWAIT_KIND_TOOL_CONFIRM,
  TOOL_CONFIRM_REPLY_APPROVE,
  TOOL_CONFIRM_REPLY_DENY,
} from "../awaitConstants";
import type { RunStatus } from "../types";

export type AwaitReplySubmitDeps = {
  resolveSessionId: () => string | undefined;
  inputText: Ref<string>;
  awaitingRunId: Ref<string>;
  awaitKind: Ref<string>;
  refreshRunStatus: () => Promise<void>;
  notifyError: (message: string) => void;
};

export function useAwaitReply() {
  const isAwaitingUser = ref(false);
  const awaitingRunId = ref("");
  const awaitKind = ref("");
  const awaitToolKey = ref("");

  function applyAwaitMeta(rs: { awaitKind?: string; awaitToolKey?: string }) {
    awaitKind.value = rs.awaitKind ?? "";
    awaitToolKey.value = rs.awaitToolKey ?? "";
  }

  function clearAwaitMeta() {
    awaitKind.value = "";
    awaitToolKey.value = "";
  }

  function applyRunStatus(rs: RunStatus) {
    isAwaitingUser.value = rs.status === "awaiting_user";
    awaitingRunId.value = rs.runId;
    if (rs.status === "awaiting_user") {
      applyAwaitMeta(rs);
    } else {
      clearAwaitMeta();
    }
  }

  function createSubmitHandlers(deps: AwaitReplySubmitDeps) {
    async function submitAwaitingReply() {
      const sid = deps.resolveSessionId();
      const text = deps.inputText.value.trim();
      if (!sid || !text) return;
      const runtime = useChatRuntimeStore();
      try {
        const ok = await runtime.submitAwaitReply(sid, text, deps.awaitingRunId.value || undefined);
        if (ok) {
          deps.inputText.value = "";
          await deps.refreshRunStatus();
        } else {
          deps.notifyError("提交回复未被接受，请重试");
        }
      } catch (err) {
        deps.notifyError(err instanceof Error ? err.message : "提交回复失败");
      }
    }

    async function submitToolConfirm(approved: boolean) {
      const sid = deps.resolveSessionId();
      if (!sid || deps.awaitKind.value !== AWAIT_KIND_TOOL_CONFIRM) return;
      const reply = approved ? TOOL_CONFIRM_REPLY_APPROVE : TOOL_CONFIRM_REPLY_DENY;
      const runtime = useChatRuntimeStore();
      try {
        const ok = await runtime.submitAwaitReply(sid, reply, deps.awaitingRunId.value || undefined);
        if (ok) {
          await deps.refreshRunStatus();
        } else {
          deps.notifyError("工具确认提交未被接受，请重试");
        }
      } catch (err) {
        deps.notifyError(err instanceof Error ? err.message : "工具确认提交失败");
      }
    }

    return { submitAwaitingReply, submitToolConfirm };
  }

  return {
    isAwaitingUser,
    awaitingRunId,
    awaitKind,
    awaitToolKey,
    applyAwaitMeta,
    clearAwaitMeta,
    applyRunStatus,
    createSubmitHandlers,
  };
}
