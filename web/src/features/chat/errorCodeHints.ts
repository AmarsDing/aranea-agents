/**
 * Error code → action hint mapping for chat ErrorBlock.
 *
 * Covers two categories of error codes:
 *   1. TurnErrorCode — chat/turn-level errors produced by the streaming pipeline
 *      (LLM_CALL_FAILED, TURN_TIMEOUT, …). These are stable frontend identifiers.
 *   2. ApiErrorCode — backend `pkg/apierror.Code` values surfaced via WS error
 *      envelopes or HTTP error responses (NOT_FOUND, BAD_REQUEST, …).
 *      See `pkg/apierror/apierror.go` for the canonical list.
 *
 * The hint drives which inline action button(s) ErrorBlock renders.
 */

export type TurnErrorCode =
  | 'AGENT_BUILD_FAILED'
  | 'ATTACHMENT_FAILED'
  | 'ATTACHMENT_UNSUPPORTED'
  | 'LLM_CALL_FAILED'
  | 'TURN_TIMEOUT'
  | 'EMPTY_REPLY'
  | 'FIRST_BYTE_TIMEOUT'
  | 'AGENT_FORBIDDEN'
  | 'STREAM_PREVIEW_FAILED';

/**
 * Backend `apierror.Code` constants. Mirrors `pkg/apierror/apierror.go` so
 * that error envelopes carrying `error.code` can be mapped without a network
 * round-trip. Keep in sync with the Go source of truth.
 */
export type ApiErrorCode =
  | 'NOT_FOUND'
  | 'BAD_REQUEST'
  | 'UNAUTHORIZED'
  | 'FORBIDDEN'
  | 'CONFLICT'
  | 'INTERNAL'
  | 'UNAVAILABLE'
  | 'RATE_LIMITED';

/** All error codes recognized by the hint lookup. */
export type ChatErrorCode = TurnErrorCode | ApiErrorCode;

export type ErrorAction =
  | 'switch_model'
  | 'retry'
  | 'rephrase'
  | 'check_config'
  | 'remove_attachment'
  | 'relogin'
  | 'none';

/** i18n keys for action labels; resolved by ErrorBlock via `t()`. */
const ACTION_LABEL_KEYS: Record<ErrorAction, string> = {
  switch_model: 'chat.errorBlock.hintSwitchModel',
  retry: 'chat.errorBlock.hintRetry',
  rephrase: 'chat.errorBlock.hintRephrase',
  check_config: 'chat.errorBlock.hintCheckConfig',
  remove_attachment: 'chat.errorBlock.hintRemoveAttachment',
  relogin: 'chat.errorBlock.hintRelogin',
  none: '',
};

/** i18n keys for button labels; resolved by ErrorBlock via `t()`. */
const ACTION_BUTTON_KEYS: Record<ErrorAction, string> = {
  switch_model: 'chat.errorBlock.btnSwitchModel',
  retry: 'chat.errorBlock.btnRetry',
  rephrase: 'chat.errorBlock.btnRephrase',
  check_config: 'chat.errorBlock.btnCheckConfig',
  remove_attachment: 'chat.errorBlock.btnRemoveAttachment',
  relogin: 'chat.errorBlock.btnRelogin',
  none: '',
};

/**
 * Error code → action mapping. Covers both TurnErrorCode and ApiErrorCode.
 * Codes not listed here fall back to `retry` when the error is recoverable
 * (HTTP 5xx / WS error) and `none` otherwise.
 */
const ERROR_CODE_HINTS: Record<ChatErrorCode, ErrorAction> = {
  // ── TurnErrorCode ──
  LLM_CALL_FAILED: 'switch_model',
  TURN_TIMEOUT: 'retry',
  FIRST_BYTE_TIMEOUT: 'switch_model',
  EMPTY_REPLY: 'rephrase',
  AGENT_BUILD_FAILED: 'check_config',
  ATTACHMENT_FAILED: 'remove_attachment',
  ATTACHMENT_UNSUPPORTED: 'remove_attachment',
  AGENT_FORBIDDEN: 'check_config',
  STREAM_PREVIEW_FAILED: 'retry',

  // ── ApiErrorCode (mirrors pkg/apierror/apierror.go) ──
  NOT_FOUND: 'retry',
  BAD_REQUEST: 'rephrase',
  UNAUTHORIZED: 'relogin',
  FORBIDDEN: 'check_config',
  CONFLICT: 'retry',
  INTERNAL: 'retry',
  UNAVAILABLE: 'retry',
  RATE_LIMITED: 'retry',
};

/** i18n key for the inline hint label of a given action (empty string for `none`). */
export function getActionHintLabelKey(action: ErrorAction): string {
  return ACTION_LABEL_KEYS[action];
}

/** i18n key for the button label of a given action (empty string for `none`). */
export function getActionButtonLabelKey(action: ErrorAction): string {
  return ACTION_BUTTON_KEYS[action];
}

/**
 * Resolve the action for a given error code.
 *
 * Returns `'none'` when the code is unknown so callers can decide whether to
 * render a generic retry button or hide actions entirely.
 */
export function getErrorAction(errorCode?: string): ErrorAction {
  if (!errorCode) return 'none';
  return ERROR_CODE_HINTS[errorCode as ChatErrorCode] ?? 'none';
}
