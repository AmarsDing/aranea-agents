/**
 * 行业 monogram 工具函数
 *
 * 取行业 key 的大写首字母作为"icon"视觉锚点，
 * 颜色由 key 字符的简单 hash 映射到行业色板。
 * 共享于 IndustryCard / IndustryDrawer / IndustryTableRow。
 */

const PALETTES = [
  'linear-gradient(135deg, #4F46E5 0%, #312E81 100%)', // indigo
  'linear-gradient(135deg, #E55C5C 0%, #9B2226 100%)', // rose
  'linear-gradient(135deg, #0EA5E9 0%, #075985 100%)', // sky
  'linear-gradient(135deg, #10B981 0%, #065F46 100%)', // emerald
  'linear-gradient(135deg, #F59E0B 0%, #92400E 100%)', // amber
  'linear-gradient(135deg, #8B5CF6 0%, #4C1D95 100%)', // violet
];

/** 根据 key hash 选择渐变色板 */
export function monoBgForKey(key: string): string {
  let h = 0;
  for (let i = 0; i < key.length; i++) {
    h = (h * 31 + key.charCodeAt(i)) | 0;
  }
  return PALETTES[Math.abs(h) % PALETTES.length];
}

/** 提取 key 前 2 个字母大写作为 monogram */
export function monoLettersForKey(key: string, fallback: string): string {
  const cleaned = key.replace(/[^a-zA-Z]/g, '').toUpperCase();
  if (cleaned.length >= 2) return cleaned.slice(0, 2);
  if (cleaned.length === 1) return cleaned + cleaned;
  return fallback.slice(0, 2);
}
