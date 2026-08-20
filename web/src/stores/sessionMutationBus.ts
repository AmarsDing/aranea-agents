import type { Agent } from '../features/agents/types';
import type { Session } from '../features/session/types';

// TECH-DEBT(S14): module-level state bypasses Pinia.
// Reason: sessionMutationBus is a cross-Store event bus used by 6+ Stores
// (session, chat/session, chat/message, agents, avatar, app) to broadcast
// session mutations without creating circular Store dependencies. Converting
// to a Pinia store would require all consumers to import that Store, re-
// introducing the circular dependency problem this bus was designed to solve.
// Migration plan: evaluate Pinia cross-store plugins (e.g. pinia-plugin-
// subscribe) or a dedicated event-bus package if the bus grows beyond
// session mutations. Issue: tracking — frontend architecture cleanup.
//
// 正名（2026-08-20）：原 sessionSync.ts 拆分为本总线与 sessionFavorites.ts，
// 名称即职责 —— 本模块只做会话/Agent 变更的跨 Store 广播。

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
      console.warn('[sessionMutationBus] subscriber error on mutation:', mutation.type, e);
    }
  }
}
