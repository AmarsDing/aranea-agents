// useSkillsPage — 行为契约：
// 1) 筛选变更时 page>1 自动归 1（useListPage watch 重置逻辑）；
// 2) 删除当前页最后一个条目后空页回退（deleteTargetSkill 的 page 回退）；
// 3) S-2 冒烟：rows/total 不再本地维护副本，直接别名 store 状态，行操作无需手动同步。
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount, flushPromises } from '@vue/test-utils';
import { defineComponent } from 'vue';
import { useSkillsPage } from '../useSkillsPage';
import { useSkillsStore } from '../../../stores/skills';
import { listSkills, toggleSkillEnabled, deleteSkill } from '../api';
import type { Skill, SkillListQuery } from '../types';

vi.mock('quasar', () => ({
  useQuasar: () => ({ notify: vi.fn(), dialog: vi.fn(() => ({ onOk: vi.fn(), onCancel: vi.fn() })) }),
}));

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}));

vi.mock('../../../stores/ecosystem', () => ({
  useEcosystemStore: () => ({ products: [] as { name: string }[], load: vi.fn(async () => {}) }),
}));

vi.mock('../api', () => ({
  listSkills: vi.fn(async () => ({ items: [], total: 0 })),
  listSkillRuns: vi.fn(async () => ({ items: [], total: 0 })),
  toggleSkillEnabled: vi.fn(),
  publishSkill: vi.fn(),
  duplicateSkill: vi.fn(),
  deleteSkill: vi.fn(async () => {}),
  uploadSkillZip: vi.fn(),
  getSkillImportJob: vi.fn(),
  refineSkillConflictGroup: vi.fn(),
  applySkillImport: vi.fn(),
  getSkillFilesystemHealth: vi.fn(async () => ({
    root_accessible: true,
    resolved_root: '/tmp/skills',
    missing_count: 0,
    pending_filesystem_count: 0,
  })),
  getSkill: vi.fn(async () => ({ skill: { id: 's1' }, bodyMarkdown: '' })),
  getSkillHealth: vi.fn(),
  listSkillFiles: vi.fn(async () => []),
  readSkillFile: vi.fn(async () => ({ path: 'SKILL.md', content: '' })),
  updateSkillFile: vi.fn(),
  createSkill: vi.fn(),
  updateSkill: vi.fn(),
  getSkillVersions: vi.fn(),
  rollbackSkillVersion: vi.fn(),
  listSkillTags: vi.fn(async () => []),
  createSkillTag: vi.fn(),
  renameSkillTag: vi.fn(),
  deleteSkillTag: vi.fn(),
}));

const skillA = { id: 's1', name: 'Alpha', enabled: true } as unknown as Skill;
const skillB = { id: 's2', name: 'Beta', enabled: true } as unknown as Skill;

function setupPage() {
  let page!: ReturnType<typeof useSkillsPage>;
  mount(
    defineComponent({
      setup() {
        page = useSkillsPage();
        return () => null;
      },
    }),
  );
  return page;
}

describe('useSkillsPage', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it('筛选变更时 page>1 自动归 1', async () => {
    vi.mocked(listSkills).mockImplementation(async (query?: SkillListQuery) => ({
      items: query?.page === 2 ? [skillB] : [skillA],
      total: 21,
    }));
    const page = setupPage();
    await flushPromises();
    expect(page.page.value).toBe(1);

    page.page.value = 2;
    await flushPromises();
    expect(vi.mocked(listSkills).mock.calls.at(-1)?.[0]?.page).toBe(2);

    page.search.value = 'foo';
    await flushPromises();
    expect(page.page.value).toBe(1);
    const lastQuery = vi.mocked(listSkills).mock.calls.at(-1)?.[0];
    expect(lastQuery?.page).toBe(1);
    expect(lastQuery?.search).toBe('foo');
  });

  it('删除当前页最后一个条目后空页回退到上一页', async () => {
    let deleted = false;
    vi.mocked(deleteSkill).mockImplementation(async () => {
      deleted = true;
    });
    vi.mocked(listSkills).mockImplementation(async (query?: SkillListQuery) => {
      if (query?.page === 2) return { items: deleted ? [] : [skillB], total: deleted ? 20 : 21 };
      return { items: [skillA], total: deleted ? 20 : 21 };
    });
    const page = setupPage();
    await flushPromises();

    page.page.value = 2;
    await flushPromises();
    expect(page.rows.value.map((s) => s.id)).toEqual(['s2']);

    page.confirmDelete(skillB);
    expect(page.deleteOpen.value).toBe(true);
    await page.deleteTargetSkill();
    await flushPromises();

    expect(deleteSkill).toHaveBeenCalledWith('s2');
    expect(page.deleteOpen.value).toBe(false);
    // 空页触发回退：page 归 1，并重新加载上一页内容
    expect(page.page.value).toBe(1);
    expect(page.rows.value.map((s) => s.id)).toEqual(['s1']);
  });

  it('S-2：rows/total 直接来自 store，行操作无需手动同步', async () => {
    vi.mocked(listSkills).mockResolvedValue({ items: [skillA], total: 1 });
    const store = useSkillsStore();
    const page = setupPage();
    await flushPromises();

    // 同一引用：composable 不再持有本地 rows/total 副本
    expect(page.rows.value).toBe(store.skills);
    expect(page.total.value).toBe(1);

    vi.mocked(toggleSkillEnabled).mockResolvedValue({ ...skillA, enabled: false } as Skill);
    await page.onToggleEnabled(skillA, false);
    expect(page.rows.value[0]?.enabled).toBe(false);
    expect(store.skills[0]?.enabled).toBe(false);
  });
});
