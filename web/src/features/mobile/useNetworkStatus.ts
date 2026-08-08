import { onScopeDispose, ref, type Ref } from 'vue';

/**
 * P3.2c: reactive network status for mobile views.
 *
 * Wraps `navigator.onLine` + window online/offline events. Assumes online in
 * non-browser environments (SSR/tests without window) so callers degrade to
 * live data. Listeners are removed when the owning scope is disposed.
 */
export function useNetworkStatus(): { online: Ref<boolean> } {
  const online = ref(typeof navigator === 'undefined' || navigator.onLine !== false);

  if (typeof window !== 'undefined' && typeof window.addEventListener === 'function') {
    const handleOnline = () => {
      online.value = true;
    };
    const handleOffline = () => {
      online.value = false;
    };
    window.addEventListener('online', handleOnline);
    window.addEventListener('offline', handleOffline);
    onScopeDispose(() => {
      window.removeEventListener('online', handleOnline);
      window.removeEventListener('offline', handleOffline);
    });
  }

  return { online };
}
