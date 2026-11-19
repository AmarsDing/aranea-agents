/** Memory Center top-level tabs. `governance` is the Trust surface; `pending` is the R3 approval-layer withheld-write list; `notes` is the kind=note user/workspace notes view (spec §6.3 wb 双 tab 形态); `ops` is admin-only. */
export const MEMORY_CENTER_TABS = ['panorama', 'graph', 'browse', 'notes', 'governance', 'pending', 'ops'] as const;

export type MemoryCenterTab = (typeof MEMORY_CENTER_TABS)[number];

/** Maps a deep-link / action-item tab onto a visible tab. Non-admins cannot stay on Ops. */
export function resolveMemoryCenterTab(raw: string, isAdmin: boolean): MemoryCenterTab {
  const tab = raw.trim();
  if (tab === 'ops') {
    return isAdmin ? 'ops' : 'governance';
  }
  if ((MEMORY_CENTER_TABS as readonly string[]).includes(tab)) {
    return tab as MemoryCenterTab;
  }
  return 'panorama';
}
