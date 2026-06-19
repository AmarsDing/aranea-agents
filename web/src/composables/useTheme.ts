/**
 * useTheme — theme management composable with auto mode support (B7).
 *
 * Supports three modes:
 *   - 'auto': follows the system `prefers-color-scheme` media query.
 *     When the system switches, the theme updates automatically.
 *   - 'dark': always dark mode.
 *   - 'light': always light mode.
 *
 * User manual selection (dark/light) overrides auto mode and is persisted
 * to localStorage. Selecting 'auto' restores system-following behavior.
 *
 * Usage:
 *   const { themeMode, isDark, setTheme, cycleTheme } = useTheme();
 *   // themeMode.value === 'auto' | 'dark' | 'light'
 *   // isDark.value === true | false (actual applied state)
 *   // setTheme('auto') — follow system
 *   // cycleTheme() — auto → dark → light → auto
 */
import { computed, ref, watch, onUnmounted } from 'vue';
import { Dark } from 'quasar';

export type ThemeMode = 'auto' | 'dark' | 'light';

const STORAGE_KEY = 'theme';

/** Shared singleton state — all components share the same theme state. */
const themeMode = ref<ThemeMode>(loadThemeMode());
let mediaQuery: MediaQueryList | null = null;
let mediaListener: ((e: MediaQueryListEvent) => void) | null = null;
let initialized = false;

function loadThemeMode(): ThemeMode {
  if (typeof localStorage === 'undefined') return 'auto';
  const raw = localStorage.getItem(STORAGE_KEY);
  if (raw === 'dark' || raw === 'light') return raw;
  return 'auto';
}

function getSystemPrefersDark(): boolean {
  if (typeof window === 'undefined' || !window.matchMedia) return false;
  return window.matchMedia('(prefers-color-scheme: dark)').matches;
}

function applyTheme(mode: ThemeMode): void {
  if (mode === 'auto') {
    Dark.set(getSystemPrefersDark());
  } else {
    Dark.set(mode === 'dark');
  }
}

function ensureMediaListener(): void {
  if (initialized || typeof window === 'undefined' || !window.matchMedia) return;
  mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
  mediaListener = () => {
    // Only react to system changes when in auto mode.
    if (themeMode.value === 'auto') {
      Dark.set(mediaQuery!.matches);
    }
  };
  mediaQuery.addEventListener('change', mediaListener);
  initialized = true;
}

function persistThemeMode(mode: ThemeMode): void {
  if (typeof localStorage === 'undefined') return;
  localStorage.setItem(STORAGE_KEY, mode);
}

/**
 * Theme management composable.
 *
 * Returns reactive `themeMode` and `isDark`, plus `setTheme` and `cycleTheme`.
 * The shared media-query listener is initialized once and shared across all
 * composable instances.
 */
export function useTheme() {
  // Initialize media listener on first use (client-side only).
  ensureMediaListener();

  // Apply the current theme on first call (ensures auto mode is applied
  // even if boot/theme.ts didn't run, e.g., in tests).
  if (!Dark.isActive && themeMode.value === 'dark') {
    applyTheme(themeMode.value);
  } else if (Dark.isActive && themeMode.value === 'light') {
    applyTheme(themeMode.value);
  } else if (themeMode.value === 'auto') {
    applyTheme('auto');
  }

  const isDark = computed(() => Dark.isActive);

  // Persist theme mode changes and apply immediately.
  const stopWatch = watch(themeMode, (mode) => {
    persistThemeMode(mode);
    applyTheme(mode);
  });

  onUnmounted(() => {
    stopWatch();
  });

  function setTheme(mode: ThemeMode): void {
    themeMode.value = mode;
  }

  /** Cycle: auto → dark → light → auto. */
  function cycleTheme(): void {
    const order: ThemeMode[] = ['auto', 'dark', 'light'];
    const idx = order.indexOf(themeMode.value);
    themeMode.value = order[(idx + 1) % order.length];
  }

  return {
    themeMode,
    isDark,
    setTheme,
    cycleTheme,
  };
}
