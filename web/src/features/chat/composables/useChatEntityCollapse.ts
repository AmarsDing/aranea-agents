import { reactive, ref } from 'vue';

const LS_SECTION_COLLAPSED = 'chat:collapsed:sections';
const LS_GROUP_COLLAPSED = 'chat:collapsed:groups';

export function useChatEntityCollapse() {
  const sectionCollapsed = reactive<{ agents: boolean; teams: boolean; activeTeams: boolean; completedTeams: boolean }>(
    {
      agents: false,
      teams: false,
      activeTeams: false,
      completedTeams: true,
    },
  );
  const groupCollapsed = reactive<Record<string, boolean>>({});

  /** Snapshot of group collapse state before search activation, used to restore on clear. */
  const groupSnapshot = ref<Record<string, boolean> | null>(null);

  function restore() {
    try {
      const raw = localStorage.getItem(LS_SECTION_COLLAPSED);
      if (raw) {
        const parsed = JSON.parse(raw) as {
          agents?: boolean;
          teams?: boolean;
          activeTeams?: boolean;
          completedTeams?: boolean;
        };
        if (typeof parsed.agents === 'boolean') sectionCollapsed.agents = parsed.agents;
        if (typeof parsed.teams === 'boolean') sectionCollapsed.teams = parsed.teams;
        if (typeof parsed.activeTeams === 'boolean') sectionCollapsed.activeTeams = parsed.activeTeams;
        if (typeof parsed.completedTeams === 'boolean') sectionCollapsed.completedTeams = parsed.completedTeams;
      }
    } catch {
      /* ignore */
    }
    try {
      const raw = localStorage.getItem(LS_GROUP_COLLAPSED);
      if (raw) {
        const parsed = JSON.parse(raw) as Record<string, boolean>;
        for (const [k, v] of Object.entries(parsed)) {
          if (typeof v === 'boolean') groupCollapsed[k] = v;
        }
      }
    } catch {
      /* ignore */
    }
  }

  function saveSections() {
    try {
      localStorage.setItem(LS_SECTION_COLLAPSED, JSON.stringify({ ...sectionCollapsed }));
    } catch {
      /* ignore */
    }
  }

  function saveGroups() {
    try {
      localStorage.setItem(LS_GROUP_COLLAPSED, JSON.stringify({ ...groupCollapsed }));
    } catch {
      /* ignore */
    }
  }

  function toggleSection(section: 'agents' | 'teams' | 'activeTeams' | 'completedTeams') {
    sectionCollapsed[section] = !sectionCollapsed[section];
    saveSections();
  }

  function toggleGroup(key: string) {
    groupCollapsed[key] = !groupCollapsed[key];
    saveGroups();
  }

  function isGroupCollapsed(key: string): boolean {
    return !!groupCollapsed[key];
  }

  function expandAllGroups() {
    for (const key of Object.keys(groupCollapsed)) {
      groupCollapsed[key] = false;
    }
    saveGroups();
  }

  /** Called when search becomes active: snapshot current group state and expand all. */
  function onSearchActive() {
    if (groupSnapshot.value === null) {
      groupSnapshot.value = { ...groupCollapsed };
    }
    expandAllGroups();
  }

  /** Called when search is cleared: restore group state from snapshot. */
  function onSearchClear() {
    if (groupSnapshot.value !== null) {
      for (const [k, v] of Object.entries(groupSnapshot.value)) {
        groupCollapsed[k] = v;
      }
      // Remove keys that weren't in the snapshot
      for (const k of Object.keys(groupCollapsed)) {
        if (!(k in groupSnapshot.value)) {
          delete groupCollapsed[k];
        }
      }
      groupSnapshot.value = null;
      saveGroups();
    }
  }

  restore();

  return {
    sectionCollapsed,
    groupCollapsed,
    toggleSection,
    toggleGroup,
    isGroupCollapsed,
    expandAllGroups,
    onSearchActive,
    onSearchClear,
  };
}
