import type { Message, MessageOrigin } from '../../domain/types';

export type { MessageOrigin };

/**
 * Derive MessageOrigin from a message ID using prefix conventions.
 *
 * This is a **migration bridge**: existing in-flight messages still carry
 * prefix-based IDs (pending-user-*, member-*, act-*, tool-*).
 * Once all message creation sites set `origin` explicitly at construction time,
 * this function can be removed and `ensureOrigin` can simply assert `message.origin`.
 *
 * Migration plan: after streamHandlers / toolEventMarkdown always set origin,
 * flip `ensureOrigin` to throw on missing origin, then remove `originFromId`.
 */
export function originFromId(id: string, role: string): MessageOrigin {
  if (id.startsWith('pending-user-')) return { kind: 'pending_user', localId: id };
  if (id.startsWith('actv-'))
    return { kind: 'streaming', sessionId: '' };
  if (id.startsWith('ws-stream-')) return { kind: 'streaming', sessionId: '' };
  if (id.startsWith('member-')) return { kind: 'team_member', agentKey: id.replace(/^member-/, '') };
  if (id.startsWith('act-') || id.startsWith('tool-')) return { kind: 'tool_activity', toolEventId: id };
  return { kind: 'persisted' };
}

export function isEphemeralOrigin(origin: MessageOrigin | undefined): boolean {
  if (!origin) return false;
  return origin.kind !== 'persisted';
}

export function isInFlightOrigin(origin: MessageOrigin | undefined): boolean {
  if (!origin) return false;
  return origin.kind === 'pending_user' || origin.kind === 'streaming' || origin.kind === 'streaming_snapshot' || origin.kind === 'tool_activity';
}

export function isPendingUserOrigin(origin: MessageOrigin | undefined): boolean {
  return origin?.kind === 'pending_user';
}

export function isStreamingOrigin(origin: MessageOrigin | undefined): boolean {
  return origin?.kind === 'streaming' || origin?.kind === 'streaming_snapshot';
}

export function isTeamMemberOrigin(origin: MessageOrigin | undefined): boolean {
  return origin?.kind === 'team_member';
}

export function isToolActivityOrigin(origin: MessageOrigin | undefined): boolean {
  return origin?.kind === 'tool_activity';
}

export function ensureOrigin(message: Message): Message {
  if (message.origin) return message;
  return { ...message, origin: originFromId(message.id, message.role) };
}
