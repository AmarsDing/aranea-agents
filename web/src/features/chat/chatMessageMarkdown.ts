import 'highlight.js/styles/github-dark-dimmed.css';
import DOMPurify from 'dompurify';
import MarkdownIt from 'markdown-it';
import { detectLanguage, highlight } from './lib/detectCodeLanguage';

const COLLAPSE_THRESHOLD = 20;

const markdown = new MarkdownIt({
  breaks: true,
  html: false,
  linkify: true,
});
markdown.enable(['table', 'strikethrough']);

let codeBlockIndex = 0;

markdown.renderer.rules.fence = (tokens, idx) => {
  const token = tokens[idx]!;
  const info = (token.info || '').trim();
  const langHint = info ? info.split(/\s+/)[0]! : '';
  const code = token.content;
  const lang = detectLanguage(code, langHint || undefined);
  const langLabel = lang === 'plaintext' ? 'code' : lang;
  const highlighted = highlight(code, lang);
  const lineCount = code.split('\n').length;
  const collapsed = lineCount > COLLAPSE_THRESHOLD;
  const index = codeBlockIndex++;

  const langClass = `hljs language-${markdown.utils.escapeHtml(lang)}`;

  if (collapsed) {
    return `<div class="code-block code-block--collapsed" data-lang="${markdown.utils.escapeHtml(lang)}">
  <div class="code-block__header">
    <span class="code-block__lang">${markdown.utils.escapeHtml(langLabel)}</span>
    <button type="button" class="code-block__copy" aria-label="复制代码">
      <span class="code-block__copy-icon" aria-hidden="true"></span>
      <span class="code-block__copy-text">复制</span>
    </button>
  </div>
  <div class="code-block__collapsed-hint" data-code-index="${index}">▶ 展开代码 (${lineCount} 行)</div>
  <div class="code-block__body" style="display:none">
    <pre><code class="${langClass}">${highlighted}</code></pre>
  </div>
  <div class="code-block__collapse-hint" data-code-index="${index}" style="display:none">收起代码</div>
</div>`;
  }

  return `<div class="code-block" data-lang="${markdown.utils.escapeHtml(lang)}">
  <div class="code-block__header">
    <span class="code-block__lang">${markdown.utils.escapeHtml(langLabel)}</span>
    <button type="button" class="code-block__copy" aria-label="复制代码">
      <span class="code-block__copy-icon" aria-hidden="true"></span>
      <span class="code-block__copy-text">复制</span>
    </button>
  </div>
  <div class="code-block__body">
    <pre><code class="${langClass}">${highlighted}</code></pre>
  </div>
</div>`;
};

export function formatMessageStamp(iso: string): string {
  if (!iso) return '';
  try {
    const d = new Date(iso);
    const now = new Date();
    const sameDay = d.toDateString() === now.toDateString();
    if (sameDay) {
      return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
    }
    const diffDays = Math.floor((now.getTime() - d.getTime()) / 86_400_000);
    if (diffDays < 7) {
      return d.toLocaleString(undefined, {
        weekday: 'short',
        hour: '2-digit',
        minute: '2-digit',
      });
    }
    return d.toLocaleString(undefined, {
      month: 'short',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    });
  } catch {
    return iso;
  }
}

export function renderChatMarkdown(content: string): string {
  return DOMPurify.sanitize(markdown.render(content || ''), {
    ADD_TAGS: ['button'],
    ADD_ATTR: ['type', 'aria-label', 'aria-hidden', 'data-lang', 'data-code-index'],
  });
}

const MD_CACHE_MAX = 400;
const mdCache = new Map<string, string>();

function markdownCacheKey(messageId: string, content: string): string {
  const id = messageId || 'anon';
  const len = content.length;
  // Use a hash-like key: combine length with head and tail to avoid collisions
  // where different content shares the same length and tail (common during streaming).
  const head = len > 48 ? content.slice(0, 48) : content;
  const tail = len > 48 ? content.slice(-48) : '';
  return `${id}:${len}:${head}:${tail}`;
}

function trimMarkdownCache() {
  while (mdCache.size > MD_CACHE_MAX) {
    const first = mdCache.keys().next().value;
    if (first === undefined) break;
    mdCache.delete(first);
  }
}

/** Cached markdown render for chat rows (avoids re-parsing 100+ messages on each WS tick).
 *
 * Unified MD rendering: streaming and completed states use the same markdown-it
 * + DOMPurify pipeline. The `streaming` parameter is preserved for API
 * compatibility but no longer branches the rendering path.
 *
 * Rationale: with the backend's 16ms delta batch window (≤60fps), markdown-it
 * parsing at 0.5-2ms/call is well within budget. The previous "escape-only"
 * fast path was an over-optimization whose premise (per-token parse) no
 * longer holds — but caused users to perceive "data arrived but no render"
 * since plain text is visibly different from MD-formatted output.
 */
export function renderChatMarkdownForMessage(messageId: string, content: string, _streaming = false): string {
  const key = markdownCacheKey(messageId, content);
  const hit = mdCache.get(key);
  if (hit !== undefined) return hit;
  const html = renderChatMarkdown(content);
  mdCache.set(key, html);
  trimMarkdownCache();
  return html;
}

export function clearChatMarkdownCache() {
  mdCache.clear();
  partsCache.clear();
}

/* ================= 流式增量渲染（块级冻结，借鉴 xai-grok-markdown checkpoint） ================= */

/**
 * 计算 content 的可冻结前缀长度（字符偏移）。
 *
 * 冻结语义：content.slice(0, end) 的独立渲染结果，与它在任何「追加更多内容后的
 * 完整文档」中的渲染结果逐字节一致。只有满足该不变量，前端才能把前缀 DOM
 * 固定下来（v-once），后续只重渲染尾部。
 *
 * 规则（CommonMark 风险清单）：
 * - 边界 = 空行（且空行之后还有非空白内容；EOF 处空行不算，块仍可能生长）
 * - 空行所在「区块」（上一空行到本空行之间）含列表标记 → 不可冻结
 *   （松散列表会回溯改变所有 item 的 <p> 包裹）
 * - 空行前最近非空行是 ≥4 空格/tab 缩进 → 不可冻结（缩进代码块吸收空行）
 * - fence（```/~~~）内的空行不产生边界；fence 未闭合时其前的边界仍有效
 * - 含链接引用定义（[label]: dest，fence 外）→ 返回 0，整条消息禁用冻结
 *   （前向引用可回溯改变已冻结区的渲染结果）
 */
export function computeFrozenPrefixEnd(content: string): number {
  if (!content) return 0;

  // 最后一个非空白字符之后的偏移；空行边界只有在其后仍有内容时才被确认
  let lastContentEnd = content.length;
  while (lastContentEnd > 0 && /\s/.test(content[lastContentEnd - 1]!)) lastContentEnd--;

  const LIST_MARKER = /^[ ]{0,3}(?:[-*+]|[0-9]{1,9}[.)])([ \t]|$)/;
  const INDENTED_CODE = /^(?:[ ]{4}|\t)/;
  const FENCE_OPEN = /^[ ]{0,3}(`{3,}|~{3,})/;
  const FENCE_CLOSE = /^[ ]{0,3}(`{3,}|~{3,})[ \t]*$/;
  const LINK_REF_DEF = /^[ ]{0,3}\[[^\]]+\]:/;

  let inFence: { ch: string; len: number } | null = null;
  let regionHasListMarker = false;
  let prevLineRisky = false; // 最近非空行是列表标记或缩进代码
  let hasLinkRefDef = false;
  let lastBoundary = 0;

  const lines = content.split('\n');
  let lineStart = 0;
  for (const raw of lines) {
    const line = raw.endsWith('\r') ? raw.slice(0, -1) : raw;
    const nextLineStart = lineStart + raw.length + 1;

    if (inFence) {
      const close = FENCE_CLOSE.exec(line);
      if (close && close[1]![0] === inFence.ch && close[1]!.length >= inFence.len) inFence = null;
      lineStart = nextLineStart;
      continue;
    }

    if (line.trim() === '') {
      const boundary = nextLineStart;
      if (lastContentEnd > boundary && !regionHasListMarker && !prevLineRisky) {
        lastBoundary = boundary;
      }
      // 空行开启新区块；列表风险不回溯跨越空行（上区块已被本边界判定）
      regionHasListMarker = false;
      prevLineRisky = false;
      lineStart = nextLineStart;
      continue;
    }

    const open = FENCE_OPEN.exec(line);
    if (open) {
      inFence = { ch: open[1]![0]!, len: open[1]!.length };
      prevLineRisky = false;
      lineStart = nextLineStart;
      continue;
    }

    if (LINK_REF_DEF.test(line)) hasLinkRefDef = true;
    const isListMarker = LIST_MARKER.test(line);
    if (isListMarker) regionHasListMarker = true;
    prevLineRisky = isListMarker || INDENTED_CODE.test(line);
    lineStart = nextLineStart;
  }

  // 引用定义可在文档任意位置定义、被前后引用，整条消息禁用冻结
  if (hasLinkRefDef) return 0;
  return lastBoundary;
}

export interface ChatMarkdownParts {
  frozenHtml: string;
  tailHtml: string;
  /** frozenHtml 的分段（每段对应一次冻结推进、追加式单调增长），
   * 供 DOM 分段渲染逐段固定，已冻结段的 DOM 节点永不触碰 */
  frozenSegments: readonly string[];
  /** 冻结世代号：整体失效（非 append-only / 边界回退）时 +1，
   * 供渲染方识别 frozenSegments 被整体替换 */
  frozenEpoch: number;
}

interface PartsState {
  /** 已确认的完整输入（append-only 校验基准） */
  content: string;
  /** 整体失效计数（组件用它做 key 前缀，保证替换后不复用旧 DOM） */
  epoch: number;
  frozenEnd: number;
  segments: string[];
  tailSource: string;
  tailHtml: string;
  result: ChatMarkdownParts;
}

const partsCache = new Map<string, PartsState>();

function trimPartsCache() {
  while (partsCache.size > MD_CACHE_MAX) {
    const first = partsCache.keys().next().value;
    if (first === undefined) break;
    partsCache.delete(first);
  }
}

function buildResult(state: PartsState): ChatMarkdownParts {
  return {
    frozenHtml: state.segments.join(''),
    tailHtml: state.tailHtml,
    frozenSegments: state.segments,
    frozenEpoch: state.epoch,
  };
}

/** 整体失效：清空冻结与尾部，epoch +1（保留条目以延续 epoch 单调性） */
function invalidateState(state: PartsState): void {
  state.epoch++;
  state.content = '';
  state.frozenEnd = 0;
  state.segments = [];
  state.tailSource = '';
  state.tailHtml = '';
  state.result = buildResult(state);
}

function freshState(content: string): PartsState {
  const state: PartsState = {
    content: '',
    epoch: 0,
    frozenEnd: 0,
    segments: [],
    tailSource: '',
    tailHtml: '',
    result: { frozenHtml: '', tailHtml: '', frozenSegments: [], frozenEpoch: 0 },
  };
  advanceState(state, content);
  return state;
}

function renderSegment(source: string): string {
  return DOMPurify.sanitize(markdown.render(source), {
    ADD_TAGS: ['button'],
    ADD_ATTR: ['type', 'aria-label', 'aria-hidden', 'data-lang', 'data-code-index'],
  });
}

/** 在 append-only 前提下推进 state 到 content */
function advanceState(state: PartsState, content: string): void {
  if (content === state.content) return;
  state.content = content;

  const newFrozenEnd = computeFrozenPrefixEnd(content);
  if (newFrozenEnd < state.frozenEnd) {
    // 边界回退（如流式途中出现链接引用定义）→ 整体失效重算
    invalidateState(state);
    state.content = content;
  }
  if (newFrozenEnd > state.frozenEnd) {
    // 只渲染新增冻结段并追加（分块渲染与整段渲染逐字节一致）
    state.segments.push(renderSegment(content.slice(state.frozenEnd, newFrozenEnd)));
    state.frozenEnd = newFrozenEnd;
  }

  const tailSource = content.slice(state.frozenEnd);
  if (tailSource !== state.tailSource) {
    state.tailSource = tailSource;
    state.tailHtml = tailSource ? renderSegment(tailSource) : '';
  }
  state.result = buildResult(state);
}

/**
 * 分段渲染：返回 { frozenHtml, tailHtml }。
 * - frozenHtml：已冻结前缀的 HTML，流式期间单调增长、绝不回改，前端可 v-once 固定
 * - tailHtml：未确认尾部的 HTML，每次输入变化重渲染
 *
 * 不变量：frozenHtml + tailHtml === renderChatMarkdownForMessage(id, content, true)
 * （由 streaming equivalence 安全网测试保障）。
 *
 * 非流式（streaming=false）走无状态全量渲染，对应 finish() 兜底语义。
 * 输入非 append-only 时整体重置（wholesale invalidation）。
 */
export function renderChatMarkdownParts(messageId: string, content: string, streaming = false): ChatMarkdownParts {
  const id = messageId || 'anon';
  if (!streaming) {
    // finish() 兜底：完成态走 mdCache 全量渲染——切换会话/重挂载后历史消息
    // 反复重渲染时不重复 parse（renderChatMarkdownForMessage 内部已缓存）。
    // 同时销毁该消息的流式增量状态（segments/tailSource 等），释放内存；
    // 若同 id 再次流式（如重新生成），从 fresh state 重新开始。
    partsCache.delete(id);
    return {
      frozenHtml: '',
      tailHtml: renderChatMarkdownForMessage(id, content, false),
      frozenSegments: [],
      frozenEpoch: 0,
    };
  }
  let state = partsCache.get(id);
  if (!state) {
    state = freshState(content);
    partsCache.set(id, state);
    trimPartsCache();
  } else {
    if (!content.startsWith(state.content)) {
      // 非 append-only（消息被重写/切换）→ 整体失效重算
      invalidateState(state);
    }
    advanceState(state, content);
  }
  return state.result;
}
