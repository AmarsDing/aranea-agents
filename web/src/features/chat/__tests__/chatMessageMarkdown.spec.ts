import { describe, expect, it, beforeEach } from 'vitest';
import { renderChatMarkdownForMessage, clearChatMarkdownCache } from '../chatMessageMarkdown';

describe('chatMessageMarkdown fence rule', () => {
  it('renders code block with language tag', () => {
    const md = '```typescript\nconst x = 1;\n```';
    const html = renderChatMarkdownForMessage('test-msg-1', md);
    expect(html).toContain('code-block');
    expect(html).toContain('data-lang="typescript"');
    expect(html).toContain('hljs');
  });

  it('renders code block without language as plaintext', () => {
    const md = '```\nsome code\n```';
    const html = renderChatMarkdownForMessage('test-msg-2', md);
    expect(html).toContain('code-block');
    expect(html).toContain('data-lang');
  });

  it('includes copy button', () => {
    const md = '```json\n{"key": "value"}\n```';
    const html = renderChatMarkdownForMessage('test-msg-3', md);
    expect(html).toContain('code-block__copy');
  });

  it('includes code-block__body with pre/code tags', () => {
    const md = '```go\nfmt.Println("hello")\n```';
    const html = renderChatMarkdownForMessage('test-msg-4', md);
    expect(html).toContain('code-block__body');
    expect(html).toContain('<pre>');
    expect(html).toContain('<code');
  });

  it('escapes HTML in code content', () => {
    const md = '```html\n<div class="test">&amp;</div>\n```';
    const html = renderChatMarkdownForMessage('test-msg-5', md);
    // The raw HTML should be escaped, not interpreted
    expect(html).not.toContain('<div class="test">');
    expect(html).toContain('&lt;div');
  });

  it('renders inline code with <code> tag', () => {
    const md = 'Use `npm install` to install';
    const html = renderChatMarkdownForMessage('test-msg-6', md);
    expect(html).toContain('<code>');
    expect(html).toContain('npm install');
  });

  it('renders long code blocks with collapse hint', () => {
    const lines = Array.from({ length: 25 }, (_, i) => `line ${i + 1}`).join('\n');
    const md = '```python\n' + lines + '\n```';
    const html = renderChatMarkdownForMessage('test-msg-7', md);
    expect(html).toContain('code-block__collapsed-hint');
  });
});

describe('renderChatMarkdownForMessage - streaming MD formatting', () => {
  beforeEach(() => clearChatMarkdownCache());

  it('renders markdown in streaming mode (code blocks formatted)', () => {
    const content = '```python\nprint("hello")\n```';
    const html = renderChatMarkdownForMessage('msg-1', content, true);
    expect(html).toContain('class="code-block');
    expect(html).toContain('print');
  });

  it('renders markdown in streaming mode (lists formatted)', () => {
    const content = '- item 1\n- item 2\n- item 3';
    const html = renderChatMarkdownForMessage('msg-2', content, true);
    expect(html).toMatch(/<ul[^>]*>/);
    expect(html).toContain('<li>item 1</li>');
  });

  it('renders markdown in streaming mode (links formatted)', () => {
    const content = 'See [docs](https://example.com) for more';
    const html = renderChatMarkdownForMessage('msg-3', content, true);
    expect(html).toContain('<a href="https://example.com"');
  });

  it('streaming and non-streaming produce same MD output for same content', () => {
    const content = '**bold** and `code` and [link](https://x.com)';
    const streaming = renderChatMarkdownForMessage('msg-4', content, true);
    clearChatMarkdownCache();
    const nonStreaming = renderChatMarkdownForMessage('msg-4', content, false);
    // 核心断言：流式和非流式结果一致（streaming 参数不影响 MD 解析）
    expect(streaming).toBe(nonStreaming);
  });
});
