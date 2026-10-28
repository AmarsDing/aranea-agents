export type ChatDraftRecord = { text: string; revision: number; updatedAt: number };
export const CHAT_DRAFT_KEY = 'aranea.chat.drafts.v1';
export const CHAT_DRAFT_RESET = 'aranea:chat-drafts-reset';

/** Tab-scoped storage only. Logout clears both persisted and live drafts. */
export function clearChatDrafts() {
  try {
    sessionStorage.removeItem(CHAT_DRAFT_KEY);
  } catch {
    /* Storage can be disabled. */
  }
  if (typeof window !== 'undefined') window.dispatchEvent(new Event(CHAT_DRAFT_RESET));
}
