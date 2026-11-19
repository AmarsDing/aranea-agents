import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { useSessionsPage } from '../useSessionsPage';
import type { Session } from '../types';

const route = vi.hoisted(() => ({ params: { sessionId: '' } }));
const store = vi.hoisted(() => ({
  searchPage: vi.fn(),
  fetchSession: vi.fn(),
  exportSession: vi.fn(),
}));

vi.mock('vue-router', () => ({ useRoute: () => route }));
vi.mock('quasar', () => ({ useQuasar: () => ({ notify: vi.fn() }) }));
vi.mock('../../../stores/session/index', () => ({ useSessionStore: () => store }));

function session(id: string) {
  return { id, title: `会话 ${id}`, status: 'active', pinned_at: '' } as unknown as Session;
}

describe('useSessionsPage inspector selection', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
    route.params.sessionId = '';
  });

  it('selects a row locally without changing the route or fetching again', async () => {
    const rows = [session('a'), session('b')];
    store.searchPage.mockResolvedValue({ items: rows, total: 2 });
    const page = useSessionsPage();
    await page.loadRows();

    page.selectSession(rows[1]);

    expect(page.selected.value?.id).toBe('b');
    expect(store.fetchSession).not.toHaveBeenCalled();
    expect(route.params.sessionId).toBe('');
  });

  it('keeps route-param loading compatible with the full detail route', async () => {
    route.params.sessionId = 'a';
    store.searchPage.mockResolvedValue({ items: [session('a')], total: 1 });
    const page = useSessionsPage();
    await page.loadRows();
    expect(page.selected.value?.id).toBe('a');
  });
});
