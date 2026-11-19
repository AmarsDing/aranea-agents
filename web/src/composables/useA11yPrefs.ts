import { reactive } from 'vue';

/**
 * 显示偏好（密度 / 色觉安全 / 动效）。
 *
 * `_nt-tokens.css` 已经定义了 `[data-density='compact']`、`[data-colorvision='safe']`、
 * `[data-motion='reduced']` 三档覆盖，但应用从未把这些属性写到 <html> 上，导致
 * 三档能力成了死代码。这里统一读写并持久化，boot/a11y.ts 在应用启动时初始化。
 */
export type A11yDensity = 'comfortable' | 'compact';
export type A11yColorVision = 'default' | 'safe';
export type A11yMotion = 'full' | 'reduced';

const STORAGE_KEYS = {
  density: 'a11y:density',
  colorvision: 'a11y:colorvision',
  motion: 'a11y:motion',
} as const;

const state = reactive({
  density: 'comfortable' as A11yDensity,
  colorvision: 'default' as A11yColorVision,
  motion: 'full' as A11yMotion,
});

let initialized = false;
let motionExplicit = false;
let mediaQuery: MediaQueryList | null = null;

function read<T extends string>(key: string, allowed: readonly T[], fallback: T): T {
  try {
    const raw = localStorage.getItem(key);
    return allowed.includes(raw as T) ? (raw as T) : fallback;
  } catch {
    return fallback;
  }
}

function write(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // 隐私模式 / 存储被禁用时忽略：偏好只在本次会话生效
  }
}

function apply() {
  if (typeof document === 'undefined') return;
  const root = document.documentElement;
  root.dataset.density = state.density;
  root.dataset.colorvision = state.colorvision;
  root.dataset.motion = state.motion;
}

export function useA11yPrefs() {
  return {
    prefs: state,
    init,
    setDensity,
    setColorvision,
    setMotion,
  };
}

export function init() {
  if (initialized || typeof window === 'undefined') return;
  initialized = true;

  state.density = read(STORAGE_KEYS.density, ['comfortable', 'compact'] as const, 'comfortable');
  state.colorvision = read(STORAGE_KEYS.colorvision, ['default', 'safe'] as const, 'default');

  const storedMotion = readStored(STORAGE_KEYS.motion);
  motionExplicit = storedMotion === 'full' || storedMotion === 'reduced';

  mediaQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
  state.motion = motionExplicit ? (storedMotion as A11yMotion) : mediaQuery.matches ? 'reduced' : 'full';
  mediaQuery.addEventListener('change', onSystemMotionChange);

  apply();
}

function readStored(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function onSystemMotionChange(event: MediaQueryListEvent) {
  if (motionExplicit) return;
  state.motion = event.matches ? 'reduced' : 'full';
  apply();
}

export function setDensity(value: A11yDensity) {
  state.density = value;
  write(STORAGE_KEYS.density, value);
  apply();
}

export function setColorvision(value: A11yColorVision) {
  state.colorvision = value;
  write(STORAGE_KEYS.colorvision, value);
  apply();
}

export function setMotion(value: A11yMotion) {
  state.motion = value;
  motionExplicit = true;
  write(STORAGE_KEYS.motion, value);
  apply();
}
