/**
 * cssVar：读取 CSS 变量（含昼夜主题覆盖）。
 *
 * 夜间令牌定义在 body.body--dark 上，必须从 body 读取才能拿到昼夜覆盖
 * （从 html 读不到 body 上的变量）。非浏览器环境（单测）返回 fallback。
 */
export function readCssVar(name: string, fallback: string): string {
  if (typeof document === 'undefined') return fallback;
  const el = document.body ?? document.documentElement;
  const v = getComputedStyle(el).getPropertyValue(name).trim();
  return v || fallback;
}
