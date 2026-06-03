export type SortableSession = {
  pinned_at?: string;
  last_message_at?: string;
  updated_at?: string;
  created_at?: string;
  at?: string;
  timeline_at?: string;
};

function sessionSortKey(session: SortableSession): number {
  const pinned = session.pinned_at?.trim();
  if (pinned) {
    const t = new Date(pinned).getTime();
    if (Number.isFinite(t)) return t;
  }
  return 0;
}

function sessionTimeValue(session: SortableSession): number {
  const raw = session.last_message_at || session.updated_at || session.created_at || session.timeline_at || session.at;
  if (!raw) return 0;
  const value = new Date(raw).getTime();
  return Number.isFinite(value) ? value : 0;
}

function sessionUpdatedValue(session: SortableSession): number {
  const raw = session.updated_at || session.created_at || session.at;
  if (!raw) return 0;
  const value = new Date(raw).getTime();
  return Number.isFinite(value) ? value : 0;
}

export function sortSessionsForDisplay<T extends SortableSession>(rows: T[]): T[] {
  return [...rows].sort((a, b) => {
    const pinDiff = sessionSortKey(b) - sessionSortKey(a);
    if (pinDiff !== 0) return pinDiff;
    const aMsg = sessionTimeValue(a);
    const bMsg = sessionTimeValue(b);
    if (aMsg !== bMsg) return bMsg - aMsg;
    return sessionUpdatedValue(b) - sessionUpdatedValue(a);
  });
}
