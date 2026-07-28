import { inject, type InjectionKey } from 'vue';
import type { useChatWorkspace } from './useChatWorkspace';

export type ChatWorkspace = ReturnType<typeof useChatWorkspace>;

/**
 * Provide/inject key for a shared ChatWorkspace instance.
 *
 * Mobile pages (sessions list / chat detail) live in separate route
 * components but must share ONE workspace (single WS stream manager, single
 * onMounted bootstrap). MobileLayout creates the workspace and provides it
 * under this key; mobile pages inject it. Desktop ChatPage creates its own
 * workspace directly and is unaffected.
 */
export const CHAT_WORKSPACE_KEY: InjectionKey<ChatWorkspace> = Symbol('chat-workspace');

export function injectChatWorkspace(): ChatWorkspace {
  const workspace = inject(CHAT_WORKSPACE_KEY, null);
  if (!workspace) {
    throw new Error('ChatWorkspace is not provided — expected an ancestor layout to provide it');
  }
  return workspace;
}
