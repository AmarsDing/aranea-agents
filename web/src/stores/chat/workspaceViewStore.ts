import { defineStore } from 'pinia';
import { onScopeDispose, reactive } from 'vue';
import { CHAT_DRAFT_RESET } from '../../features/chat/chatDraftPersistence';

export interface ChatWorkspaceView {
  pane: 'chat' | 'navigation' | 'artifacts';
  navigation: 'sessions' | 'members' | 'tree';
  contextOpen: boolean;
  contextTab: 'artifacts' | 'activity';
  readingExpanded: boolean;
  preview: { id: string; version?: number } | null;
  chatScrollTop: number | null;
  artifactScrollTop: number;
  listScrollTop: number;
  processTaskId: string | null;
  processTurnId: string | null;
}
export const newWorkspaceView = (): ChatWorkspaceView => ({
  pane: 'chat',
  navigation: 'sessions',
  contextOpen: false,
  contextTab: 'artifacts',
  readingExpanded: false,
  preview: null,
  chatScrollTop: null,
  artifactScrollTop: 0,
  listScrollTop: 0,
  processTaskId: null,
  processTurnId: null,
});

/** UI positions only. Business entities and authorization remain in their existing stores. */
export const useChatWorkspaceViewStore = defineStore('chat-workspace-view', () => {
  const records = reactive(new Map<string, ChatWorkspaceView>());
  function ensure(key: string) {
    if (!records.has(key)) {
      if (records.size >= 100) records.delete(records.keys().next().value!);
      records.set(key, newWorkspaceView());
    }
    return records.get(key)!;
  }
  const clear = () => records.clear();
  if (typeof window !== 'undefined') {
    window.addEventListener(CHAT_DRAFT_RESET, clear);
    onScopeDispose(() => window.removeEventListener(CHAT_DRAFT_RESET, clear));
  }
  return { records, ensure, clear };
});
