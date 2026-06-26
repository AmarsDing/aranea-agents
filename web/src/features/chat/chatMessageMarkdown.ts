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
}
