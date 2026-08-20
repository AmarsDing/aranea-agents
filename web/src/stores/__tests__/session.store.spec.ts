import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useSessionStore } from '../session';

vi.mock('../../features/session/api', () => ({
  searchSessions: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  getSession: vi.fn(),
  createSession: vi.fn(),
  deleteSession: vi.fn().mockResolvedValue(undefined),
  archiveSession: vi.fn().mockResolvedValue(undefined),
  updateSession: vi.fn(),
  listSessionTurns: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  getSessionTimeline: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  listSessionChatMessages: vi.fn().mockResolvedValue({ items: [], currentRevision: 0 }),
  listSessionRuns: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  listSessionParticipants: vi.fn().mockResolvedValue([]),
  previewSessionBatch: vi.fn().mockResolvedValue({ count: 0, ids: [] }),
  batchArchiveSessions: vi.fn().mockResolvedValue({ affected: 0 }),
  batchDeleteSessions: vi.fn().mockResolvedValue({ affected: 0 }),
  pinSession: vi.fn(),
  unpinSession: vi.fn(),
  restoreSession: vi.fn(),
  exportSession: vi.fn().mockResolvedValue(''),
}));

vi.mock('../sessionMutationBus', () => ({
  emitSessionMutation: vi.fn(),
  onSessionMutation: vi.fn(() => vi.fn()),
}));

const mockSession = {
  id: 'sess-1',
  title: 'Test Session',
  status: 'active',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};
const mockSession2 = {
  id: 'sess-2',
  title: 'Another Session',
  status: 'active',
  created_at: '2024-01-02T00:00:00Z',
  updated_at: '2024-01-02T00:00:00Z',
};

describe('useSessionStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  // ── loadSessions ──────────────────────────────────────────────
  describe('loadSessions', () => {
    it('fills sessions and total on success', async () => {
      const { searchSessions } = await import('../../features/session/api');
      (searchSessions as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
        items: [mockSession, mockSession2],
        total: 2,
      });

      const store = useSessionStore();
      await store.loadSessions({ limit: 10 });

      expect(store.sessions).toHaveLength(2);
      expect(store.sessions[0].id).toBe('sess-1');
      expect(store.total).toBe(2);
      expect(store.loading).toBe(false);
      expect(store.error).toBeNull();
    });

    it('sets error on failure', async () => {
      const { searchSessions } = await import('../../features/session/api');
      (searchSessions as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('Network error'));

      const store = useSessionStore();
      await store.loadSessions();

      expect(store.error).toBe('Network error');
      expect(store.loading).toBe(false);
      expect(store.sessions).toHaveLength(0);
    });
  });

  // ── newSession ────────────────────────────────────────────────
  describe('newSession', () => {
    it('prepends session and sets activeSession on success', async () => {
      const { createSession } = await import('../../features/session/api');
      (createSession as ReturnType<typeof vi.fn>).mockResolvedValueOnce(mockSession);

      const store = useSessionStore();
      store.sessions = [mockSession2 as any];

      const result = await store.newSession({ agent_id: 'a-1' });

      expect(store.sessions).toHaveLength(2);
      expect(store.sessions[0].id).toBe('sess-1');
      expect(store.activeSession?.id).toBe('sess-1');
      expect(result.id).toBe('sess-1');
    });

    it('sets error on failure', async () => {
      const { createSession } = await import('../../features/session/api');
      (createSession as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('Create failed'));

      const store = useSessionStore();
      await expect(store.newSession({ agent_id: 'a-1' })).rejects.toThrow('Create failed');
      expect(store.error).toBe('Create failed');
    });
  });

  // ── removeSession ─────────────────────────────────────────────
  describe('removeSession', () => {
    it('removes session from list', async () => {
      const store = useSessionStore();
      store.sessions = [mockSession, mockSession2] as any;

      await store.removeSession('sess-1');

      expect(store.sessions).toHaveLength(1);
      expect(store.sessions[0].id).toBe('sess-2');
    });

    it('clears activeSession if it is the removed one', async () => {
      const store = useSessionStore();
      store.sessions = [mockSession] as any;
      store.activeSession = mockSession as any;

      await store.removeSession('sess-1');

      expect(store.activeSession).toBeNull();
    });

    it('keeps activeSession if it is a different session', async () => {
      const store = useSessionStore();
      store.sessions = [mockSession, mockSession2] as any;
      store.activeSession = mockSession2 as any;

      await store.removeSession('sess-1');

      expect(store.activeSession?.id).toBe('sess-2');
    });
  });

  // ── archive ───────────────────────────────────────────────────
  describe('archive', () => {
    it('removes session from list', async () => {
      const store = useSessionStore();
      store.sessions = [mockSession, mockSession2] as any;

      await store.archive('sess-1');

      expect(store.sessions).toHaveLength(1);
      expect(store.sessions[0].id).toBe('sess-2');
    });

    it('clears activeSession if it is the archived one', async () => {
      const store = useSessionStore();
      store.sessions = [mockSession] as any;
      store.activeSession = mockSession as any;

      await store.archive('sess-1');

      expect(store.activeSession).toBeNull();
    });
  });

  // ── rename ────────────────────────────────────────────────────
  describe('rename', () => {
    it('updates session title in list', async () => {
      const { updateSession } = await import('../../features/session/api');
      const updated = { ...mockSession, title: 'Renamed' };
      (updateSession as ReturnType<typeof vi.fn>).mockResolvedValueOnce(updated);

      const store = useSessionStore();
      store.sessions = [mockSession] as any;

      const result = await store.rename('sess-1', 'Renamed');

      expect(store.sessions[0].title).toBe('Renamed');
      expect(result.title).toBe('Renamed');
    });

    it('updates activeSession if it is the renamed one', async () => {
      const { updateSession } = await import('../../features/session/api');
      const updated = { ...mockSession, title: 'Renamed' };
      (updateSession as ReturnType<typeof vi.fn>).mockResolvedValueOnce(updated);

      const store = useSessionStore();
      store.sessions = [mockSession] as any;
      store.activeSession = mockSession as any;

      await store.rename('sess-1', 'Renamed');

      expect(store.activeSession?.title).toBe('Renamed');
    });
  });

  // ── setPinned ─────────────────────────────────────────────────
  describe('setPinned', () => {
    it('pins a session', async () => {
      const { pinSession } = await import('../../features/session/api');
      const pinned = { ...mockSession, pinned: true };
      (pinSession as ReturnType<typeof vi.fn>).mockResolvedValueOnce(pinned);

      const store = useSessionStore();
      store.sessions = [mockSession] as any;

      const result = await store.setPinned('sess-1', true);

      expect(store.sessions[0].pinned).toBe(true);
      expect(result.pinned).toBe(true);
    });

    it('unpins a session', async () => {
      const { unpinSession } = await import('../../features/session/api');
      const unpinned = { ...mockSession, pinned: false };
      (unpinSession as ReturnType<typeof vi.fn>).mockResolvedValueOnce(unpinned);

      const store = useSessionStore();
      store.sessions = [{ ...mockSession, pinned: true }] as any;

      const result = await store.setPinned('sess-1', false);

      expect(store.sessions[0].pinned).toBe(false);
      expect(result.pinned).toBe(false);
    });

    it('updates activeSession if it is the target', async () => {
      const { pinSession } = await import('../../features/session/api');
      const pinned = { ...mockSession, pinned: true };
      (pinSession as ReturnType<typeof vi.fn>).mockResolvedValueOnce(pinned);

      const store = useSessionStore();
      store.sessions = [mockSession] as any;
      store.activeSession = mockSession as any;

      await store.setPinned('sess-1', true);

      expect(store.activeSession?.pinned).toBe(true);
    });
  });

  // ── removeSessionLocal ────────────────────────────────────────
  describe('removeSessionLocal', () => {
    it('removes session from list without API call', async () => {
      const { deleteSession } = await import('../../features/session/api');

      const store = useSessionStore();
      store.sessions = [mockSession, mockSession2] as any;

      store.removeSessionLocal('sess-1');

      expect(store.sessions).toHaveLength(1);
      expect(store.sessions[0].id).toBe('sess-2');
      expect(deleteSession).not.toHaveBeenCalled();
    });

    it('clears activeSession if it is the removed one', () => {
      const store = useSessionStore();
      store.sessions = [mockSession] as any;
      store.activeSession = mockSession as any;

      store.removeSessionLocal('sess-1');

      expect(store.activeSession).toBeNull();
    });
  });

  // ── updateSessionLocal ────────────────────────────────────────
  describe('updateSessionLocal', () => {
    it('replaces session in list', () => {
      const store = useSessionStore();
      store.sessions = [mockSession, mockSession2] as any;

      const updated = { ...mockSession, title: 'Updated' };
      store.updateSessionLocal('sess-1', updated as any);

      expect(store.sessions).toHaveLength(2);
      expect(store.sessions[0].title).toBe('Updated');
    });

    it('updates activeSession if it is the target', () => {
      const store = useSessionStore();
      store.sessions = [mockSession] as any;
      store.activeSession = mockSession as any;

      const updated = { ...mockSession, title: 'Updated' };
      store.updateSessionLocal('sess-1', updated as any);

      expect(store.activeSession?.title).toBe('Updated');
    });

    it('does not change activeSession if it is a different session', () => {
      const store = useSessionStore();
      store.sessions = [mockSession, mockSession2] as any;
      store.activeSession = mockSession2 as any;

      const updated = { ...mockSession, title: 'Updated' };
      store.updateSessionLocal('sess-1', updated as any);

      expect(store.activeSession?.id).toBe('sess-2');
      expect(store.activeSession?.title).toBe('Another Session');
    });
  });
});
