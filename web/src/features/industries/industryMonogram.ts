/**
 * 行业 monogram 工具函数
 *
 * 取行业 key 的大写首字母作为"icon"视觉锚点，
 * 颜色由 key 字符的简单 hash 映射到行业色板。
 * 共享于 IndustryCard / IndustryDrawer / IndustryTableRow。
 */

const PALETTES = ['indigo', 'rose', 'sky', 'emerald', 'amber', 'violet'] as const;

/** 根据 key hash 选择渐变色板，从 CSS 变量读取颜色值 */
export function monoBgForKey(key: string): string {
  let h = 0;
  for (let i = 0; i < key.length; i++) {
    h = (h * 31 + key.charCodeAt(i)) | 0;
  }
  const idx = Math.abs(h) % PALETTES.length;
  const s = getComputedStyle(document.documentElement).getPropertyValue(`--palette-industry-${idx}-start`).trim();
  const e = getComputedStyle(document.documentElement).getPropertyValue(`--palette-industry-${idx}-end`).trim();
  return `linear-gradient(135deg, ${s} 0%, ${e} 100%)`;
}

/** 提取 key 前 2 个字母大写作为 monogram */
export function monoLettersForKey(key: string, fallback: string): string {
  const cleaned = key.replace(/[^a-zA-Z]/g, '').toUpperCase();
  if (cleaned.length >= 2) return cleaned.slice(0, 2);
  if (cleaned.length === 1) return cleaned + cleaned;
  return fallback.slice(0, 2);
}
