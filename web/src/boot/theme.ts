import { defineBoot } from '#q-app/wrappers';
import { Dark } from 'quasar';

/**
 * B7: Theme boot — supports 'auto' | 'dark' | 'light' modes.
 *
 * - 'auto' (default): follows system `prefers-color-scheme`. The media-query
 *   listener is set up in `useTheme` composable; here we only apply the
 *   initial state.
 * - 'dark' / 'light': explicit user preference, overrides system.
 *
 * The `useTheme` composable manages the media-query listener for reactive
 * system changes. This boot file only sets the initial state before the app
 * mounts.
 */
export default defineBoot(() => {
  if (import.meta.env.SSR) return;
  const raw = localStorage.getItem('theme');
  if (raw === 'dark') {
    Dark.set(true);
  } else if (raw === 'light') {
    Dark.set(false);
  } else {
    // 'auto' or unset — follow system preference.
    const prefersDark =
      typeof window !== 'undefined' && window.matchMedia
        ? window.matchMedia('(prefers-color-scheme: dark)').matches
        : false;
    Dark.set(prefersDark);
  }
});
