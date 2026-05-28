import { useI18n } from "vue-i18n";

export type TurnErrorCode =
  | "AGENT_BUILD_FAILED"
  | "ATTACHMENT_FAILED"
  | "ATTACHMENT_UNSUPPORTED"
  | "LLM_CALL_FAILED"
  | "TURN_TIMEOUT"
  | "EMPTY_REPLY"
  | "FIRST_BYTE_TIMEOUT"
  | "AGENT_FORBIDDEN"
  | "STREAM_PREVIEW_FAILED";

export interface ErrorActionHint {
  action: "switch_model" | "retry" | "rephrase" | "check_config" | "remove_attachment" | "none";
  label: string;
}

const ACTION_LABELS: Record<ErrorActionHint["action"], string> = {
  switch_model: "chat.errorHint.switchModel",
  retry: "chat.errorHint.retry",
  rephrase: "chat.errorHint.rephrase",
  check_config: "chat.errorHint.checkConfig",
  remove_attachment: "chat.errorHint.removeAttachment",
  none: "",
};

const ERROR_CODE_HINTS: Record<string, ErrorActionHint["action"]> = {
  LLM_CALL_FAILED: "switch_model",
  TURN_TIMEOUT: "retry",
  FIRST_BYTE_TIMEOUT: "switch_model",
  EMPTY_REPLY: "rephrase",
  AGENT_BUILD_FAILED: "check_config",
  ATTACHMENT_FAILED: "remove_attachment",
  ATTACHMENT_UNSUPPORTED: "remove_attachment",
  AGENT_FORBIDDEN: "check_config",
  STREAM_PREVIEW_FAILED: "retry",
};

export function getErrorActionHint(errorCode?: string): ErrorActionHint | null {
  if (!errorCode) return null;
  const action = ERROR_CODE_HINTS[errorCode];
  if (!action) return null;
  return { action, label: ACTION_LABELS[action] };
}

export function formatErrorWithHint(message: string, errorCode?: string): string {
  const hint = getErrorActionHint(errorCode);
  if (!hint || !hint.label) return message;
  return `${message}（${hint.label}）`;
}
