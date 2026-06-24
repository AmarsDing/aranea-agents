import type { Agent } from '../features/agents/types';
import type { Session } from '../features/session/types';
import { ref } from 'vue';

// TECH-DEBT(S14): module-level state bypasses Pinia.
// Reason: sessionSync is a cross-Store event bus used by 6+ Stores
// (session, chat/session, chat/message, agents, avatar, app) to broadcast
// session mutations without creating circular Store dependencies. Converting
// to a Pinia store would require all consumers to import that Store, re-
// introducing the circular dependency problem this bus was designed to solve.
// favoriteSessionIDs is a localStorage-backed Set consumed by useChatWorkspace
// only; keeping it here avoids a dedicated Store for a single boolean toggle.
// Migration plan: evaluate Pinia cross-store plugins (e.g. pinia-plugin-
// subscribe) or a dedicated event-bus package if the bus grows beyond
// session mutations. Issue: tracking — frontend architecture cleanup.

type SessionMutation =
  | { type: 'remove'; id: string }
  | { type: 'update'; id: string; session: Session }
  | { type: 'archive'; id: string }
  | { type: 'refresh' }
  | { type: 'status_changed'; id: string; status: string; statusReason: string; statusChangedAt: string }
  | { type: 'agent_removed'; agentId: string }
  | { type: 'agent_updated'; agent: Agent }
  | { type: 'agents_dependencies_loaded' };

type MutationHandler = (mutation: SessionMutation) => void;

const listeners: MutationHandler[] = [];

export function onSessionMutation(handler: MutationHandler): () => void {
  listeners.push(handler);
  return () => {
    const idx = listeners.indexOf(handler);
    if (idx >= 0) listeners.splice(idx, 1);
  };
}

export function emitSessionMutation(mutation: SessionMutation): void {
  for (const handler of listeners) {
    try {
      handler(mutation);
    } catch (e) {
      console.warn('[sessionSync] subscriber error on mutation:', mutation.type, e);
    }
  }
}

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
