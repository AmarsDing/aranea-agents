import { defineBoot } from '#q-app/wrappers';
import { init } from '../composables/useA11yPrefs';

/**
 * 把显示偏好写到 <html data-density / data-colorvision / data-motion>，
 * 让 _nt-tokens.css 里已定义的三档覆盖生效（紧凑密度 / 色觉安全 / 减少动效）。
 * 未显式设置时，动效跟随系统 prefers-reduced-motion。
 */
export default defineBoot(() => {
  if (import.meta.env.SSR) return;
  init();
});
