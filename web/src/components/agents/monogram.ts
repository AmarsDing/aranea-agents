/**
 * Monogram：取 key 的前两个字母大写作视觉锚点（组织架构的公司页签在用）。
 */

/** 提取 key 前 2 个字母大写作为 monogram；key 没有字母时退回名称前两个字。 */
export function monoLettersForKey(key: string, fallback: string): string {
  const cleaned = key.replace(/[^a-zA-Z]/g, '').toUpperCase();
  if (cleaned.length >= 2) return cleaned.slice(0, 2);
  if (cleaned.length === 1) return cleaned + cleaned;
  return fallback.slice(0, 2);
}
