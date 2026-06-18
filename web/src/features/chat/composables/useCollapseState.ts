import { ref, type Ref } from 'vue';

/**
 * T8.4: Manage collapse/expand state with sessionStorage persistence.
 *
 * State is remembered per-key across page refreshes and component re-mounts,
 * so users don't lose their expand/collapse choices when the message list
 * re-renders (e.g., due to virtual scrolling recycling or session reload).
 *
 * Design notes:
 * - sessionStorage (not localStorage): collapse state is per-session, not
 *   permanent. Clearing the session resets to defaults.
 * - Silent fallback: if sessionStorage is unavailable (private mode, quota),
 *   the composable still works as an in-memory ref.
 * - User vs system operations: only `toggle()` (user-initiated) persists to
 *   sessionStorage. `setCollapsed()` is for system-driven state changes
 *   (e.g., streaming auto-collapse, threshold auto-collapse) and does NOT
 *   persist, so it won't overwrite the user's saved preference.
 */
const STORAGE_PREFIX = 'chat:collapse:';

export function useCollapseState(
  key: string,
  defaultCollapsed: boolean = true,
): {
  collapsed: Ref<boolean>;
  toggle: () => void;
  setCollapsed: (value: boolean) => void;
} {
  const storageKey = `${STORAGE_PREFIX}${key}`;

  function readFromStorage(): boolean | null {
    try {
      const value = sessionStorage.getItem(storageKey);
      if (value === 'true') return true;
      if (value === 'false') return false;
      return null;
    } catch {
      return null;
    }
  }

  function writeToStorage(value: boolean): void {
    try {
      sessionStorage.setItem(storageKey, String(value));
    } catch {
      // sessionStorage might be unavailable (private mode, quota exceeded).
      // Silently degrade to in-memory only.
    }
  }

  const stored = readFromStorage();
  const collapsed = ref(stored !== null ? stored : defaultCollapsed);

  function toggle(): void {
    // User-initiated: persist the new state so it survives re-mounts/refreshes.
    collapsed.value = !collapsed.value;
    writeToStorage(collapsed.value);
  }

  function setCollapsed(value: boolean): void {
    // System-driven change (e.g., streaming auto-collapse, threshold
    // auto-collapse): update current state but do NOT persist, so the
    // user's saved preference (or the initial default) is preserved.
    collapsed.value = value;
  }

  return {
    collapsed,
    toggle,
    setCollapsed,
  };
}
