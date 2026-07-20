import { beforeEach, describe, expect, it } from 'vitest';
import {
  detectLanguage,
  highlight,
  escapeHtml,
  clearCodeHighlightCaches,
  resetCodeHighlightStats,
  codeHighlightStats,
} from '../lib/detectCodeLanguage';

describe('detectLanguage', () => {
  it('returns hint when it is a registered language', () => {
    expect(detectLanguage('const x = 1', 'typescript')).toBe('typescript');
    expect(detectLanguage('print(1)', 'python')).toBe('python');
  });

  it('falls back to auto-detect when hint is not a candidate', () => {
    const result = detectLanguage('const x = 1', 'brainfuck');
    // Should auto-detect as one of the candidate languages or plaintext
    expect(typeof result).toBe('string');
    expect(result.length).toBeGreaterThan(0);
  });

  it('returns plaintext for large files (>10KB) without hint', () => {
    const bigCode = 'x'.repeat(10 * 1024 + 1);
    expect(detectLanguage(bigCode)).toBe('plaintext');
  });

  it('honors hint even for large files', () => {
    const bigCode = 'x'.repeat(10 * 1024 + 1);
    expect(detectLanguage(bigCode, 'typescript')).toBe('typescript');
  });

  it('returns plaintext when auto-detect relevance is low', () => {
    // Random gibberish should not match any language with high relevance
    const result = detectLanguage('asdf qwer zxcv');
    expect(result).toBe('plaintext');
  });

  it('auto-detects common languages', () => {
    const tsCode = 'function greet(name: string): void { console.log(`Hello ${name}`); }';
    const result = detectLanguage(tsCode);
    // Should detect as typescript or javascript
    expect(['typescript', 'javascript']).toContain(result);
  });
});

describe('highlight', () => {
  it('returns escaped HTML for plaintext', () => {
    const code = '<div>hello</div>';
    const result = highlight(code, 'plaintext');
    expect(result).not.toContain('<div>');
    expect(result).toContain('&lt;div&gt;');
  });

  it('highlights TypeScript code', () => {
    const code = 'const x: number = 1;';
    const result = highlight(code, 'typescript');
    // highlight.js wraps tokens in <span class="hljs-...">
    expect(result).toContain('hljs');
  });

  it('falls back to escapeHtml on unknown language', () => {
    const code = '<b>bold</b>';
    const result = highlight(code, 'nonexistent_lang_xyz');
    expect(result).toContain('&lt;b&gt;');
  });
});

describe('escapeHtml', () => {
  it('escapes all HTML special characters', () => {
    expect(escapeHtml('<div class="x">&amp;</div>')).toBe('&lt;div class=&quot;x&quot;&gt;&amp;amp;&lt;/div&gt;');
  });

  it('escapes single quotes', () => {
    expect(escapeHtml("it's")).toBe('it&#039;s');
  });

  it('returns unchanged text without special characters', () => {
    expect(escapeHtml('hello world')).toBe('hello world');
  });
});

describe('highlight memo（流式重复渲染优化）', () => {
  beforeEach(() => {
    clearCodeHighlightCaches();
    resetCodeHighlightStats();
  });

  it('相同 code+lang 第二次调用命中缓存且结果一致', () => {
    const code = 'const x: number = 1;';
    const first = highlight(code, 'typescript');
    const second = highlight(code, 'typescript');
    expect(second).toBe(first);
    expect(codeHighlightStats.highlightMisses).toBe(1);
    expect(codeHighlightStats.highlightHits).toBe(1);
  });

  it('不同 lang 不共享缓存项', () => {
    highlight('const x = 1', 'typescript');
    highlight('const x = 1', 'javascript');
    expect(codeHighlightStats.highlightMisses).toBe(2);
    expect(codeHighlightStats.highlightHits).toBe(0);
  });

  it('plaintext 路径同样被缓存', () => {
    const a = highlight('plain <text>', 'plaintext');
    const b = highlight('plain <text>', 'plaintext');
    expect(b).toBe(a);
    expect(codeHighlightStats.highlightHits).toBe(1);
    expect(codeHighlightStats.highlightMisses).toBe(1);
  });

  it('超过大小上限的代码不写入缓存', () => {
    const big = 'const a = 1;\n'.repeat(4000); // >32KB
    expect(big.length).toBeGreaterThan(32 * 1024);
    highlight(big, 'typescript');
    highlight(big, 'typescript');
    expect(codeHighlightStats.highlightHits).toBe(0);
    expect(codeHighlightStats.highlightMisses).toBe(2);
  });

  it('LRU 容量上限驱逐最旧项', () => {
    for (let i = 0; i < 120; i++) highlight(`code ${i}`, 'plaintext');
    expect(codeHighlightStats.highlightMisses).toBe(120);
    const hitsBefore = codeHighlightStats.highlightHits;
    // 最近写入的仍命中
    highlight('code 119', 'plaintext');
    expect(codeHighlightStats.highlightHits).toBe(hitsBefore + 1);
    // 最旧的已被驱逐 → 不再命中
    highlight('code 0', 'plaintext');
    expect(codeHighlightStats.highlightHits).toBe(hitsBefore + 1);
  });

  it('缓存不影响高亮结果正确性', () => {
    const code = 'function f() { return 1; }';
    clearCodeHighlightCaches();
    const uncached = highlight(code, 'typescript');
    highlight(code, 'typescript'); // 写入缓存
    const cached = highlight(code, 'typescript'); // 命中缓存
    expect(cached).toBe(uncached);
    expect(cached).toContain('hljs');
  });
});

describe('detectLanguage memo', () => {
  beforeEach(() => {
    clearCodeHighlightCaches();
    resetCodeHighlightStats();
  });

  it('无 hint 时相同代码第二次调用命中缓存', () => {
    const code = 'function greet(name: string): void { console.log(name); }';
    const a = detectLanguage(code);
    const b = detectLanguage(code);
    expect(b).toBe(a);
    expect(codeHighlightStats.detectMisses).toBe(1);
    expect(codeHighlightStats.detectHits).toBe(1);
  });

  it('流式增长：前 500 字符 sample 稳定后持续命中缓存', () => {
    // 模拟未闭合代码块流式增长：sample（前 500 字符）稳定后，后续 chunk 不再重复 highlightAuto
    let content = 'function f() {\n' + '  const x = 1;\n'.repeat(40); // >500 chars
    detectLanguage(content); // 首次 miss
    expect(codeHighlightStats.detectMisses).toBe(1);
    for (let i = 0; i < 5; i++) {
      content += `\n  const y${i} = ${i};`;
      detectLanguage(content);
    }
    expect(codeHighlightStats.detectMisses).toBe(1); // 不再新增 miss
    expect(codeHighlightStats.detectHits).toBe(5);
  });

  it('显式 hint 不消耗 detect 缓存', () => {
    detectLanguage('const x = 1', 'typescript');
    detectLanguage('const x = 1', 'typescript');
    expect(codeHighlightStats.detectHits + codeHighlightStats.detectMisses).toBe(0);
  });

  it('大文件（>10KB）走 plaintext 快路径且不写入缓存', () => {
    const big = 'x'.repeat(11 * 1024);
    expect(detectLanguage(big)).toBe('plaintext');
    expect(detectLanguage(big)).toBe('plaintext');
    expect(codeHighlightStats.detectHits + codeHighlightStats.detectMisses).toBe(0);
  });
});
