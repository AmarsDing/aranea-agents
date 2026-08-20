import { ref } from 'vue';

// 会话收藏夹（localStorage 持久化）。原 sessionSync.ts 拆分而来（2026-08-20 正名）：
// 收藏状态与跨 Store 事件总线无关联，独立成模块。消费方仅 useChatWorkspace，
// 不建专门 Store 以避免为单一布尔开关引入 Pinia 仪式。

const FAVORITE_KEY = 'chat:favorite-sessions';

function loadFavoriteIDs(): string[] {
  try {
    const value = JSON.parse(localStorage.getItem(FAVORITE_KEY) || '[]');
    return Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : [];
  } catch {
    return [];
  }
}

function saveFavoriteIDs(ids: Set<string>) {
  localStorage.setItem(FAVORITE_KEY, JSON.stringify([...ids]));
}

export const favoriteSessionIDs = ref(new Set(loadFavoriteIDs()));

export function isFavoriteSession(id: string): boolean {
  return favoriteSessionIDs.value.has(id);
}

export function toggleFavoriteSession(id: string): void {
  const next = new Set(favoriteSessionIDs.value);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  favoriteSessionIDs.value = next;
  saveFavoriteIDs(next);
}
