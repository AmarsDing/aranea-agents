/**
 * Runs 列表数字格式化（纯函数，locale 中立）。
 * 设计目标：运维扫读时一眼分数量级，而不是数位数。
 */

/** 紧凑整数：<10,000 千分位原样（1,234），≥10,000 缩写（156.8k / 2.3M） */
export function formatCompactInt(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return '0';
  if (n < 10000) return Math.round(n).toLocaleString('en-US');
  if (n < 1_000_000) {
    const v = n / 1000;
    return `${trimOneDecimal(v)}k`;
  }
  return `${trimOneDecimal(n / 1_000_000)}M`;
}

function trimOneDecimal(v: number): string {
  const s = v.toFixed(1);
  return s.endsWith('.0') ? s.slice(0, -2) : s;
}

/**
 * 成本（USD）：0 → $0.00；极小 → <$0.01；<1 → 4 位小数（$0.0220）；≥1 → 2 位小数。
 */
export function formatCostUsd(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return '$0.00';
  if (n < 0.01) return '<$0.01';
  if (n < 1) return `$${n.toFixed(4)}`;
  return `$${n.toFixed(2)}`;
}
