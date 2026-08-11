// frontmatter（SP2 §SP2-8）：frontmatter 区段只读解析（纯函数，简化 YAML——键值 + 行内/块状列表）。
export type FrontmatterEntry = {
  key: string;
  /** 列表值逗号连接；标量原样 */
  value: string;
  /** 列表型值（tags/aliases 高亮用） */
  values: string[];
};

export type Frontmatter = {
  entries: FrontmatterEntry[];
  /** 原始区段文本（不含 --- 围栏） */
  raw: string;
};

const INLINE_LIST_RE = /^\[(.*)\]$/;

function splitInlineList(s: string): string[] {
  const m = INLINE_LIST_RE.exec(s.trim());
  if (!m) return [];
  return m[1]
    .split(',')
    .map((x) => x.trim().replace(/^['"]|['"]$/g, ''))
    .filter(Boolean);
}

/** 解析 frontmatter；无区段返回 null。仅支持一层 key: value 与 `- item` 块状列表（只读展示足够）。 */
export function parseFrontmatter(content: string): Frontmatter | null {
  if (!content.startsWith('---\n') && !content.startsWith('---\r\n')) return null;
  const start = content.indexOf('\n') + 1;
  const end = content.indexOf('\n---', start);
  if (end < 0) return null;
  const raw = content.slice(start, end);

  const entries: FrontmatterEntry[] = [];
  let current: FrontmatterEntry | null = null;
  for (const line of raw.split(/\r?\n/)) {
    if (!line.trim() || line.trim().startsWith('#')) continue;
    const kv = /^([A-Za-z_][\w-]*)\s*:\s*(.*)$/.exec(line);
    if (kv) {
      const inline = splitInlineList(kv[2]);
      current = {
        key: kv[1],
        value: inline.length ? inline.join(', ') : kv[2].trim(),
        values: inline,
      };
      entries.push(current);
      continue;
    }
    const li = /^\s*-\s+(.+)$/.exec(line);
    if (li && current) {
      const v = li[1].trim().replace(/^['"]|['"]$/g, '');
      current.values.push(v);
      current.value = current.values.join(', ');
    }
  }
  return { entries, raw };
}
