// outline（SP2 §SP2-8）：ATX 标题解析（纯函数，排除代码块内 # 行）。
export type OutlineItem = {
  /** 标题级别 1-6 */
  level: number;
  /** 标题文本（去尾部闭合 #） */
  text: string;
  /** 文档偏移（编辑器 scrollToOffset 定位用） */
  offset: number;
  /** 行号（0 基） */
  line: number;
};

const HEADING_RE = /^(#{1,6})\s+(.+?)\s*#*\s*$/;
const FENCE_RE = /^\s*(```|~~~)/;

export function parseOutline(content: string): OutlineItem[] {
  const items: OutlineItem[] = [];
  let inFence = false;
  let offset = 0;
  const lines = content.split('\n');
  for (let i = 0; i < lines.length; i++) {
    const l = lines[i];
    if (FENCE_RE.test(l)) {
      inFence = !inFence;
    } else if (!inFence) {
      const m = HEADING_RE.exec(l);
      if (m) items.push({ level: m[1].length, text: m[2], offset, line: i });
    }
    offset += l.length + 1;
  }
  return items;
}
