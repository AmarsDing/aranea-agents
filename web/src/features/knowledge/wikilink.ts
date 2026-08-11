// wikilink（SP2 §SP2-6）：[[target]] / [[target|alias]] / [[target#heading]] 解析、存在性判定、CM 补全工厂。
// 分层纪律：解析/匹配为纯函数可单测；CM 依赖仅补全工厂一处。
import type { CompletionContext, CompletionResult } from '@codemirror/autocomplete';

export type WikiLinkRef = {
  /** 链接目标（文档名，可能不含扩展名） */
  target: string;
  /** `|alias` 展示别名（无则空串） */
  alias: string;
  /** `#heading` 锚点（无则空串） */
  heading: string;
  /** 在文档中的起止偏移（含 `[[` `]]`） */
  from: number;
  to: number;
};

const WIKILINK_RE = /\[\[([^\]|#]+?)(?:#([^\]|]*?))?(?:\|([^\]]*?))?\]\]/g;

/** 全文扫描 wikilink（含代码块内——Live Preview 装饰层负责排除代码行，纯函数不感知语法）。 */
export function parseWikiLinks(doc: string): WikiLinkRef[] {
  const out: WikiLinkRef[] = [];
  for (const m of doc.matchAll(WIKILINK_RE)) {
    const target = (m[1] ?? '').trim();
    if (!target) continue;
    out.push({
      target,
      heading: (m[2] ?? '').trim(),
      alias: (m[3] ?? '').trim(),
      from: m.index ?? 0,
      to: (m.index ?? 0) + m[0].length,
    });
  }
  return out;
}

/** 芯片展示文本：alias ?? target。 */
export function wikiLinkLabel(r: Pick<WikiLinkRef, 'target' | 'alias'>): string {
  return r.alias || r.target;
}

/** 归一化文档名：去目录前缀 + 去 .md/.markdown 扩展名 + 小写（候选与目标同口径比较）。 */
export function normalizeTargetName(p: string): string {
  const segs = p.split('/').filter(Boolean);
  const base = segs.length ? segs[segs.length - 1] : p;
  return base
    .replace(/\.(md|markdown)$/i, '')
    .trim()
    .toLowerCase();
}

/** 目标存在性：候选为当前库文档（relPath 或文件名均可，归一化后比较）。 */
export function resolveWikiTarget(target: string, candidates: string[]): boolean {
  const want = normalizeTargetName(target);
  if (!want) return false;
  return candidates.some((c) => normalizeTargetName(c) === want);
}

/** CM 补全工厂：检测 `[[` 前缀，候选 = 当前库文档名（150ms 防抖由 autocompletion 配置层控制）。
 *  P2-5：`[[target#partial` 时切换为标题补全（getHeadings 提供目标文档标题列表，通常为已打开 tab 的大纲）。 */
export function wikiLinkCompletionSource(getCandidates: () => string[], getHeadings?: (target: string) => string[]) {
  return (ctx: CompletionContext): CompletionResult | null => {
    const m = ctx.matchBefore(/\[\[([^\]|#]*?)(?:#([^\]|]*))?$/);
    if (!m) return null;
    const hashIdx = m.text.indexOf('#');
    if (hashIdx >= 0) {
      // 标题段补全：插入点在 # 之后，候选 = 目标文档标题
      if (!getHeadings) return null;
      const target = m.text.slice(2, hashIdx);
      const partial = m.text.slice(hashIdx + 1);
      const want = partial.trim().toLowerCase();
      const options = getHeadings(target)
        .filter((h) => !want || h.toLowerCase().includes(want))
        .slice(0, 20)
        .map((h) => ({ label: h, detail: target }));
      if (!options.length) return null;
      return { from: ctx.pos - partial.length, options, validFor: /^[^\]|]*$/ };
    }
    const query = m.text.slice(2);
    const want = normalizeTargetName(query);
    const options = getCandidates()
      .filter((c) => !want || normalizeTargetName(c).includes(want))
      .slice(0, 20)
      .map((c) => ({ label: normalizeTargetName(c), detail: c }));
    if (!options.length) return null;
    return { from: ctx.pos - query.length, options, validFor: /^[^\]|#]*$/ };
  };
}
