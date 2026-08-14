import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useSkillsStore } from '../skills';

vi.mock('../../features/skills/api', () => ({
  listSkills: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  listSkillRuns: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  toggleSkillEnabled: vi.fn(),
  publishSkill: vi.fn(),
  duplicateSkill: vi.fn(),
  deleteSkill: vi.fn().mockResolvedValue(undefined),
  uploadSkillZip: vi.fn(),
  getSkillImportJob: vi.fn(),
  refineSkillConflictGroup: vi.fn(),
  applySkillImport: vi.fn(),
  getSkillFilesystemHealth: vi.fn(),
  getSkill: vi.fn().mockResolvedValue({ skill: { id: 's1' }, bodyMarkdown: '' }),
  getSkillHealth: vi.fn(),
  listSkillFiles: vi.fn().mockResolvedValue([]),
  readSkillFile: vi.fn().mockResolvedValue({ path: 'SKILL.md', content: '' }),
  updateSkillFile: vi.fn(),
  createSkill: vi.fn(),
  updateSkill: vi.fn(),
  getSkillVersions: vi.fn(),
  rollbackSkillVersion: vi.fn(),
  listSkillTags: vi.fn().mockResolvedValue([]),
  createSkillTag: vi.fn(),
  renameSkillTag: vi.fn(),
  deleteSkillTag: vi.fn(),
}));

describe('useSkillsStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('instantiates and exposes all actions (smoke: catches unresolved identifiers)', () => {
    const store = useSkillsStore();
    const actions = [
      'loadSkills', 'loadSkillRuns', 'toggle', 'publish', 'duplicate', 'remove',
      'loadFilesystemHealth', 'loadSkillHealth', 'loadSkill', 'uploadSkillZip',
      'getSkillImportJob', 'refineSkillConflictGroup', 'applySkillImport',
      'listSkillFiles', 'readSkillFile', 'updateSkillFile', 'create', 'update',
      'loadVersions', 'rollbackVersion',
      'loadSkillTags', 'createTag', 'renameTag', 'deleteTag', 'invalidateSkillTags',
    ] as const;
    for (const name of actions) {
      expect(typeof store[name], `store.${name} should be a function`).toBe('function');
    }
  });

  it('listSkillFiles/readSkillFile delegate to api', async () => {
    const { listSkillFiles, readSkillFile } = await import('../../features/skills/api');
    const store = useSkillsStore();
    await store.listSkillFiles('s1');
    expect(listSkillFiles).toHaveBeenCalledWith('s1');
    await store.readSkillFile('s1', 'SKILL.md');
    expect(readSkillFile).toHaveBeenCalledWith('s1', 'SKILL.md');
  });

  it('remove/duplicate keep total in sync', async () => {
    const { listSkills, duplicateSkill } = await import('../../features/skills/api');
    (listSkills as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      items: [{ id: 's1' }],
      total: 1,
    });
    (duplicateSkill as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ id: 's2' });

    const store = useSkillsStore();
    await store.loadSkills();
    expect(store.total).toBe(1);

    await store.duplicate('s1');
    expect(store.total).toBe(2);

    await store.remove('s2');
    expect(store.total).toBe(1);
    expect(store.skills.map((s) => s.id)).toEqual(['s1']);
  });
});
