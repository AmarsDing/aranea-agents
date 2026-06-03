/** Await run kinds — aligned with api/kratos/chat/v1 RunStatus.await_kind */
export const AWAIT_KIND_REPLY = 'reply';
export const AWAIT_KIND_TOOL_CONFIRM = 'tool_confirm';

/** Structured tool confirmation replies — aligned with internal/tools/serviceawaitreply */
export const TOOL_CONFIRM_REPLY_APPROVE = '__aranea:tool_confirm:approve';
export const TOOL_CONFIRM_REPLY_DENY = '__aranea:tool_confirm:deny';
