import { describe, expect, it } from 'vitest';
import { detectLanguage, highlight, escapeHtml } from '../lib/detectCodeLanguage';

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
    expect(escapeHtml('<div class="x">&amp;</div>')).toBe(
      '&lt;div class=&quot;x&quot;&gt;&amp;amp;&lt;/div&gt;',
    );
  });

  it('escapes single quotes', () => {
    expect(escapeHtml("it's")).toBe('it&#039;s');
  });

  it('returns unchanged text without special characters', () => {
    expect(escapeHtml('hello world')).toBe('hello world');
  });
});
