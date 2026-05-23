import { ref, type Ref } from "vue";
import { awaitUserReply } from "../api";
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
      const ok = await awaitUserReply(sid, text, deps.awaitingRunId.value || undefined);
      if (ok) {
        deps.inputText.value = "";
        await deps.refreshRunStatus();
      }
    }

    async function submitToolConfirm(approved: boolean) {
      const sid = deps.resolveSessionId();
      if (!sid || deps.awaitKind.value !== AWAIT_KIND_TOOL_CONFIRM) return;
      const reply = approved ? TOOL_CONFIRM_REPLY_APPROVE : TOOL_CONFIRM_REPLY_DENY;
      const ok = await awaitUserReply(sid, reply, deps.awaitingRunId.value || undefined);
      if (ok) {
        await deps.refreshRunStatus();
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
