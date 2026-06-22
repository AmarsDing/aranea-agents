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

export function detectLanguage(code: string, hint?: string): string {
  // 1. Explicit hint takes priority
  if (hint && LANG_CANDIDATES.includes(hint)) return hint;

  // 2. Large file protection
  if (code.length > 10 * 1024) return 'plaintext';

  // 3. Auto-detect with limited candidates
  const sample = code.slice(0, 500);
  const result = hljs.highlightAuto(sample, LANG_CANDIDATES);
  if (result.relevance < 5 || !result.language) return 'plaintext';

  return result.language;
}

export function highlight(code: string, lang: string): string {
  if (lang === 'plaintext') return escapeHtml(code);
  try {
    return hljs.highlight(code, { language: lang, ignoreIllegals: true }).value;
  } catch {
    return escapeHtml(code);
  }
}

export function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}
