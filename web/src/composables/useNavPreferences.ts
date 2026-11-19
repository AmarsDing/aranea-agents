import { ref } from 'vue';

const FAVORITES_KEY = 'nav:favorites';
const RECENT_KEY = 'nav:recent';
const FAVORITES_LIMIT = 8;
const RECENT_LIMIT = 5;

function readPaths(key: string): string[] {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(key) ?? '[]');
    if (!Array.isArray(parsed)) return [];
    return [...new Set(parsed.filter((value): value is string => typeof value === 'string' && value.startsWith('/')))];
  } catch {
    return [];
  }
}

function writePaths(key: string, paths: string[]) {
  try {
    localStorage.setItem(key, JSON.stringify(paths));
  } catch {
    // 存储不可用时保留本次会话状态。
  }
}

export function useNavPreferences() {
  const favorites = ref(readPaths(FAVORITES_KEY).slice(0, FAVORITES_LIMIT));
  const recent = ref(readPaths(RECENT_KEY).slice(0, RECENT_LIMIT));

  function toggleFavorite(path: string) {
    favorites.value = favorites.value.includes(path)
      ? favorites.value.filter((item) => item !== path)
      : [path, ...favorites.value].slice(0, FAVORITES_LIMIT);
    writePaths(FAVORITES_KEY, favorites.value);
  }

  function recordVisit(path: string) {
    if (!path.startsWith('/')) return;
    recent.value = [path, ...recent.value.filter((item) => item !== path)].slice(0, RECENT_LIMIT);
    writePaths(RECENT_KEY, recent.value);
  }

  function isFavorite(path: string) {
    return favorites.value.includes(path);
  }

  return { favorites, recent, toggleFavorite, recordVisit, isFavorite };
}
