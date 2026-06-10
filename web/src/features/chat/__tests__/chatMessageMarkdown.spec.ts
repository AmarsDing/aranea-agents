import { describe, expect, it } from 'vitest';
import { renderChatMarkdownForMessage } from '../chatMessageMarkdown';

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
