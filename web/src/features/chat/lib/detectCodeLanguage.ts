import hljs from 'highlight.js/lib/core';
import typescript from 'highlight.js/lib/languages/typescript';
import javascript from 'highlight.js/lib/languages/javascript';
import go from 'highlight.js/lib/languages/go';
import python from 'highlight.js/lib/languages/python';
import bash from 'highlight.js/lib/languages/bash';
import json from 'highlight.js/lib/languages/json';
import yaml from 'highlight.js/lib/languages/yaml';
import sql from 'highlight.js/lib/languages/sql';
import rust from 'highlight.js/lib/languages/rust';
import java from 'highlight.js/lib/languages/java';
import markdown from 'highlight.js/lib/languages/markdown';
import shell from 'highlight.js/lib/languages/shell';

// Register languages
hljs.registerLanguage('typescript', typescript);
hljs.registerLanguage('javascript', javascript);
hljs.registerLanguage('go', go);
hljs.registerLanguage('python', python);
hljs.registerLanguage('bash', bash);
hljs.registerLanguage('json', json);
hljs.registerLanguage('yaml', yaml);
hljs.registerLanguage('sql', sql);
hljs.registerLanguage('rust', rust);
hljs.registerLanguage('java', java);
hljs.registerLanguage('markdown', markdown);
hljs.registerLanguage('shell', shell);

const LANG_CANDIDATES = [
  'typescript',
  'javascript',
  'go',
  'python',
  'bash',
  'json',
  'yaml',
  'sql',
  'rust',
  'java',
  'markdown',
  'shell',
];

// --- Highlight memo（借鉴 Grok OpenCodeHighlighter 的持久化思想） ---
// 流式期间未闭合代码块在 tail 中每次 delta 都被完整重渲染；已闭合未冻结的块、
// 以及多处同时渲染同一内容（ThinkingBlock + ChatReasoningDrawer）都会重复调用
// detectLanguage/highlight。hljs 不暴露增量 ParseState（不同于 syntect），因此采用
// 结果级 LRU memo：key 精确为入参，命中即跳过 hljs 计算。
const HIGHLIGHT_CACHE_MAX = 100;
const HIGHLIGHT_CACHE_CODE_LIMIT = 32 * 1024;
const DETECT_CACHE_MAX = 100;
const DETECT_SAMPLE_LEN = 500;

const highlightCache = new Map<string, string>();
const detectCache = new Map<string, string>();

/** 命中/未命中计数，供测试与性能观测使用 */
export const codeHighlightStats = {
  highlightHits: 0,
  highlightMisses: 0,
  detectHits: 0,
  detectMisses: 0,
};

export function resetCodeHighlightStats(): void {
  codeHighlightStats.highlightHits = 0;
  codeHighlightStats.highlightMisses = 0;
  codeHighlightStats.detectHits = 0;
  codeHighlightStats.detectMisses = 0;
}

export function clearCodeHighlightCaches(): void {
  highlightCache.clear();
  detectCache.clear();
}

function lruGet(cache: Map<string, string>, key: string): string | undefined {
  const value = cache.get(key);
  if (value !== undefined) {
    // 刷新 recency：重新插入到 Map 尾部
    cache.delete(key);
    cache.set(key, value);
  }
  return value;
}

function lruSet(cache: Map<string, string>, key: string, value: string, max: number): void {
  if (cache.has(key)) cache.delete(key);
  cache.set(key, value);
  while (cache.size > max) {
    const oldest = cache.keys().next().value;
    if (oldest === undefined) break;
    cache.delete(oldest);
  }
}

export function detectLanguage(code: string, hint?: string): string {
  // 1. Explicit hint takes priority（O(1)，无需缓存）
  if (hint && LANG_CANDIDATES.includes(hint)) return hint;

  // 2. Large file protection（快路径，不写入缓存）
  if (code.length > 10 * 1024) return 'plaintext';

  // 3. Auto-detect with limited candidates —— memo key 为 sample：
  //    检测行为只依赖前 500 字符，流式增长中 sample 稳定后可持续命中
  const sample = code.slice(0, DETECT_SAMPLE_LEN);
  const cached = lruGet(detectCache, sample);
  if (cached !== undefined) {
    codeHighlightStats.detectHits++;
    return cached;
  }
  codeHighlightStats.detectMisses++;

  const result = hljs.highlightAuto(sample, LANG_CANDIDATES);
  const lang = result.relevance < 5 || !result.language ? 'plaintext' : result.language;
  lruSet(detectCache, sample, lang, DETECT_CACHE_MAX);
  return lang;
}

function doHighlight(code: string, lang: string): string {
  if (lang === 'plaintext') return escapeHtml(code);
  try {
    return hljs.highlight(code, { language: lang, ignoreIllegals: true }).value;
  } catch {
    return escapeHtml(code);
  }
}

export function highlight(code: string, lang: string): string {
  // 超大代码不缓存（单个缓存项内存上限 32KB，避免缓存本身成为内存压力）；
  // miss 语义 = 实际执行高亮计算的次数，绕过缓存同样计入
  if (code.length > HIGHLIGHT_CACHE_CODE_LIMIT) {
    codeHighlightStats.highlightMisses++;
    return doHighlight(code, lang);
  }

  const key = `${lang}\n${code}`;
  const cached = lruGet(highlightCache, key);
  if (cached !== undefined) {
    codeHighlightStats.highlightHits++;
    return cached;
  }
  codeHighlightStats.highlightMisses++;

  const out = doHighlight(code, lang);
  lruSet(highlightCache, key, out, HIGHLIGHT_CACHE_MAX);
  return out;
}

export function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}
