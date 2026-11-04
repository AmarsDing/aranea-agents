import { beforeEach, describe, expect, it } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { useChatWorkspaceViewStore } from '../workspaceViewStore';
import { clearChatDrafts } from '../../../features/chat/chatDraftPersistence';

beforeEach(() => setActivePinia(createPinia()));
describe('workspace view cache', () => {
  it('defaults to task progress and preserves filters, expansion and scroll across chat round trips', () => {
    const store = useChatWorkspaceViewStore();
    const state = store.ensure(JSON.stringify(['backend', 'user', 'S']));
    expect(state.pane).toBe('activity');
    state.situationQuery = { status: 'waiting', offset: 20, limit: 20, sort_by: 'updated_at', sort_order: 'desc' };
    state.situationExpandedTaskIds = ['task-1', 'task-2'];
    state.situationScrollTop = 160;
    state.pane = 'chat';
    state.pane = 'activity';
    expect(state.situationQuery).toMatchObject({ status: 'waiting', offset: 20 });
    expect(state.situationExpandedTaskIds).toEqual(['task-1', 'task-2']);
    expect(state.situationScrollTop).toBe(160);
  });

  it('retains IDs and positions without storing business content', () => {
    const store = useChatWorkspaceViewStore();
    const key = JSON.stringify(['backend', 'user', 'S']);
    const state = store.ensure(key);
    state.preview = { id: 'file', version: 0 };
    state.chatScrollTop = 0;
    state.artifactScrollTop = 140;
    expect(store.ensure(key)).toMatchObject({
      preview: { id: 'file', version: 0 },
      chatScrollTop: 0,
      artifactScrollTop: 140,
    });
    expect(store.ensure(JSON.stringify(['backend', 'other', 'S'])).preview).toBeNull();
    expect(store.ensure(JSON.stringify(['other-backend', 'user', 'S'])).preview).toBeNull();
  });
  it('clears on logout and bounds retained sessions', () => {
    const store = useChatWorkspaceViewStore();
    for (let i = 0; i < 110; i++) store.ensure(String(i));
    expect(store.records.size).toBe(100);
    clearChatDrafts();
    expect(store.records.size).toBe(0);
  });
});
