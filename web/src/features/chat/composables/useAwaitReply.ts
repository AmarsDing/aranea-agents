import { ref } from "vue";
import type { RunStatus } from "../types";

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

  return {
    isAwaitingUser,
    awaitingRunId,
    awaitKind,
    awaitToolKey,
    applyAwaitMeta,
    clearAwaitMeta,
    applyRunStatus
  };
}
