import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import type { Ref } from "vue";
import { awaitUserReply } from "../api";
import {
  TOOL_CONFIRM_REPLY_APPROVE,
  TOOL_CONFIRM_REPLY_DENY,
} from "../awaitConstants";

export type AwaitReplyDeps = {
  selectedTeamId: Ref<string | null>;
  teamSelectedSessionId: Ref<string | null>;
  selectedSessionId: Ref<string | undefined>;
  inputText: Ref<string>;
  isAwaitingUser: Ref<boolean>;
  awaitingRunId: Ref<string>;
  runStatus: Ref<string>;
  refreshRunStatus: () => Promise<void>;
};

export function useAwaitReply(deps: AwaitReplyDeps) {
  const { t } = useI18n();
  const $q = useQuasar();

  function sessionId(): string {
    return deps.selectedTeamId.value
      ? deps.teamSelectedSessionId.value ?? ""
      : deps.selectedSessionId.value ?? "";
  }

  async function sendAwaitReply(reply: string) {
    const sid = sessionId();
    if (!sid || !deps.isAwaitingUser.value) return;
    try {
      const ok = await awaitUserReply(sid, reply, deps.awaitingRunId.value || undefined);
      if (ok) {
        deps.inputText.value = "";
        deps.isAwaitingUser.value = false;
        deps.runStatus.value = "running";
        $q.notify({ type: "positive", message: t("chat.awaitReplySent") });
        void deps.refreshRunStatus();
      } else {
        $q.notify({ type: "warning", message: t("chat.awaitReplyRestartHint") });
      }
    } catch (err) {
      $q.notify({
        type: "negative",
        message: err instanceof Error ? err.message : t("chat.awaitReplyFailed"),
      });
    }
  }

  async function submitAwaitingReply() {
    const reply = deps.inputText.value.trim();
    if (!reply) return;
    await sendAwaitReply(reply);
  }

  async function submitToolConfirm(approved: boolean) {
    const token = approved ? TOOL_CONFIRM_REPLY_APPROVE : TOOL_CONFIRM_REPLY_DENY;
    await sendAwaitReply(token);
  }

  return { submitAwaitingReply, submitToolConfirm, sendAwaitReply };
}
