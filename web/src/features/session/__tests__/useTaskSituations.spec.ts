import { defineComponent, h } from 'vue';
import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { createMemoryHistory, createRouter } from 'vue-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { listMock } = vi.hoisted(() => ({ listMock: vi.fn() }));
vi.mock('../../../services/index', () => ({
  createSessionService: () => ({}),
  createSessionV2Service: () => ({ ListTaskSituations: listMock }),
}));

import { useTaskSituations } from '../useTaskSituations';

beforeEach(() => {
  setActivePinia(createPinia());
  listMock.mockReset();
  listMock.mockResolvedValue({ items: [], total: 0, limit: 20, offset: 0 });
});

describe('useTaskSituations route-driven server queries', () => {
  it('restores allow-listed filters, drops unknown keys, and reloads after pagination', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/overview', component: { template: '<div />' } },
        { path: '/task-workspace', component: { template: '<div />' } },
      ],
    });
    await router.push('/overview?status=waiting&keyword=release&limit=20&offset=20&secret=discard');
    await router.isReady();
    let view!: ReturnType<typeof useTaskSituations>;
    const wrapper = mount(
      defineComponent({
        setup() {
          view = useTaskSituations();
          return () => h('div');
        },
      }),
      { global: { plugins: [router] } },
    );
    await flushPromises();
    expect(listMock).toHaveBeenCalledWith({
      sessionId: undefined,
      status: 'waiting',
      keyword: 'release',
      limit: 20,
      offset: 20,
      sortBy: 'updated_at',
      sortOrder: 'desc',
    });
    expect(view.query.value).toEqual({
      status: 'waiting',
      keyword: 'release',
      limit: 20,
      offset: 20,
      sort_by: 'updated_at',
      sort_order: 'desc',
    });

    await view.setQuery({ status: 'failed', offset: 0 });
    await flushPromises();
    expect(router.currentRoute.value.path).toBe('/overview');
    expect(router.currentRoute.value.query).toMatchObject({ status: 'failed', offset: '0', keyword: 'release' });
    expect(router.currentRoute.value.query.secret).toBeUndefined();
    expect(listMock).toHaveBeenLastCalledWith(expect.objectContaining({ status: 'failed', offset: 0 }));
    wrapper.unmount();
  });
});
