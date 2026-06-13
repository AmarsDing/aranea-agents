const STORAGE_KEY = 'activity_first';

/** Activity-First rendering mode. When enabled, the chat UI consumes Activity events
 *  from the backend instead of inferring from messages.
 *  Set localStorage activity_first=false to disable. */
export function useActivityFirstEnabled(): boolean {
  if (typeof localStorage === 'undefined') return true;
  return localStorage.getItem(STORAGE_KEY) !== 'false';
}

export function setActivityFirstEnabled(enabled: boolean) {
  if (typeof localStorage === 'undefined') return;
  localStorage.setItem(STORAGE_KEY, enabled ? 'true' : 'false');
}
