import { beforeEach, describe, expect, it } from 'vitest';
import { useNavPreferences } from '../../composables/useNavPreferences';

describe('导航收藏与最近访问', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('损坏的本地数据回退为空集合', () => {
    localStorage.setItem('nav:favorites', '{broken');
    localStorage.setItem('nav:recent', JSON.stringify([1, null, '/sessions']));

    const prefs = useNavPreferences();

    expect(prefs.favorites.value).toEqual([]);
    expect(prefs.recent.value).toEqual(['/sessions']);
  });

  it('收藏可切换、去重且最多保留八项', () => {
    const prefs = useNavPreferences();

    for (const path of ['/one', '/two', '/three', '/four', '/five', '/six', '/seven', '/eight', '/nine']) {
      prefs.toggleFavorite(path);
    }
    expect(prefs.favorites.value).toEqual(['/nine', '/eight', '/seven', '/six', '/five', '/four', '/three', '/two']);
    expect(prefs.isFavorite('/one')).toBe(false);

    prefs.toggleFavorite('/nine');
    expect(prefs.isFavorite('/nine')).toBe(false);
    expect(prefs.favorites.value).toHaveLength(7);
  });

  it('最近访问按最新排序、去重且最多保留五项', () => {
    const prefs = useNavPreferences();

    for (const path of ['/one', '/two', '/three', '/two', '/four', '/five', '/six']) {
      prefs.recordVisit(path);
    }

    expect(prefs.recent.value).toEqual(['/six', '/five', '/four', '/two', '/three']);
  });

  it('收藏与最近访问跨实例持久化', () => {
    const first = useNavPreferences();
    first.toggleFavorite('/approvals');
    first.recordVisit('/sessions');

    const restored = useNavPreferences();

    expect(restored.favorites.value).toEqual(['/approvals']);
    expect(restored.recent.value).toEqual(['/sessions']);
  });
});
