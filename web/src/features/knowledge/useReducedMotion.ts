import { getCurrentInstance, onBeforeUnmount, ref, type Ref } from 'vue';

/**
 * prefers-reduced-motion 判定（SP2 降级契约，FR-SP2-10）。
 * matchMedia 不可用（SSR/测试环境）时降级为 false（保留动效由 CSS media query 兜底关闭）。
 */
export function useReducedMotion(): { reducedMotion: Ref<boolean> } {
  const reducedMotion = ref(false);
  if (typeof matchMedia !== 'function') {
    return { reducedMotion };
  }
  const mql = matchMedia('(prefers-reduced-motion: reduce)');
  reducedMotion.value = mql.matches;
  const onChange = (e: MediaQueryListEvent) => {
    reducedMotion.value = e.matches;
  };
  mql.addEventListener('change', onChange);
  if (getCurrentInstance()) {
    onBeforeUnmount(() => mql.removeEventListener('change', onChange));
  }
  return { reducedMotion };
}

/** 设备性能分级：粒子数量档位（FR-SP2-10 低端降级）。 */
export function particleBudget(): number {
  if (typeof navigator === 'undefined') return 60;
  const cores = navigator.hardwareConcurrency ?? 4;
  const mem = (navigator as Navigator & { deviceMemory?: number }).deviceMemory ?? 8;
  if (cores <= 2 || mem <= 2) return 0;
  if (cores <= 4 || mem <= 4) return 60;
  return 120;
}
