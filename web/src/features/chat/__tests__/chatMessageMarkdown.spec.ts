import { describe, it, expect, beforeEach, vi } from 'vitest';
import MarkdownIt from 'markdown-it';
import {
  renderChatMarkdown,
  renderChatMarkdownForMessage,
  renderChatMarkdownParts,
  computeFrozenPrefixEnd,
  clearChatMarkdownCache,
} from '../chatMessageMarkdown';

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

/**
 * 流式渲染等价性安全网（借鉴 xai-grok-markdown 的 exhaustive split-point 测试）。
 *
 * 不变量：无论 delta 如何切分到达，流式渲染的中间态（streaming=true）与
 * 终态（streaming=false，即 finish() 兜底语义）都必须与一次性全量渲染
 * 逐字节一致。当前全量重渲染实现平凡满足；引入块级冻结/增量渲染后，
 * 本套件为回归防线——冻结规则（松散列表、参考式链接定义、未闭合 fence 等）
 * 的任何错误都会在这里暴露。
 */
describe('chatMessageMarkdown streaming equivalence (safety net)', () => {
  beforeEach(() => clearChatMarkdownCache());

  /** 代表性文档：覆盖全部块类型 + 行内样式 + CJK/emoji + 对抗性用例 */
  const DOC = [
    '# 标题 Heading',
    '',
    '首段包含 **粗体**、*斜体*、`行内代码`、[链接](https://example.com) 以及 emoji 🎉 和中文混排。',
    '',
    'Setext 标题',
    '==========',
    '',
    '## 小节',
    '',
    '- 无序项一',
    '- 无序项二',
    '  - 嵌套项',
    '',
    '1. 有序甲',
    '2. 有序乙',
    '',
    '松散列表：',
    '',
    '- 松项一',
    '',
    '- 松项二',
    '',
    '> 引用第一行',
    '> 引用第二行',
    '',
    '```js',
    'function hello() {',
    '  return "world"; // 注释 🎉',
    '}',
    '```',
    '',
    '缩进代码块：',
    '',
    '    indented = code',
    '    第二行',
    '',
    '    空行后仍在代码块内',
    '',
    '| 列A | 列B |',
    '| --- | --- |',
    '| 1 | 2 |',
    '',
    '---',
    '',
    '结尾段落，裸链 https://example.com/path?a=b&c=d 收尾。',
  ].join('\n');

  /** 参考式链接文档：定义可被后向引用，冻结实现必须整体禁用或正确处理 */
  const REF_DOC = [
    '前向引用：[甲][r1] 与 [乙][r2]。',
    '',
    '中间段落 **加粗**。',
    '',
    '[r1]: https://ref.example.com/a',
    '[r2]: https://ref.example.com/b',
    '',
    '后向引用：[丙][r1]。',
  ].join('\n');

  /** 短文档：用于 4 段切分组合测试（控制组合规模） */
  const SHORT_DOC = '# 标题\n\n段落 **加粗** 文本。\n\n- 项一\n- 项二\n\n```\ncode\n```\n\n> 引用\n';

  /** 码点边界（不会切在 UTF-16 代理对中间） */
  function codePointBoundaries(s: string): number[] {
    const pts: number[] = [0];
    let idx = 0;
    for (const ch of s) {
      idx += ch.length;
      pts.push(idx);
    }
    return pts;
  }

  /** 按循环尺寸序列切 chunk */
  function chunksBySizes(s: string, sizes: number[]): string[] {
    const out: string[] = [];
    let i = 0;
    let k = 0;
    while (i < s.length) {
      const n = sizes[k++ % sizes.length];
      out.push(s.slice(i, i + n));
      i += n;
    }
    return out;
  }

  /**
   * 模拟流式到达：逐 chunk 累积并以 streaming=true 渲染，
   * 断言最后一次流式渲染 == 全量渲染（增量必须精确，不得依赖兜底），
   * 再断言 streaming=false 的终态渲染 == 全量渲染（finish() 兜底一致）。
   */
  function simulateStreaming(messageId: string, full: string, chunks: string[], expected: string) {
    let acc = '';
    let lastStreaming = '';
    for (const c of chunks) {
      acc += c;
      lastStreaming = renderChatMarkdownForMessage(messageId, acc, true);
    }
    expect(acc).toBe(full);
    expect(lastStreaming).toBe(expected);
    expect(renderChatMarkdownForMessage(messageId, acc, false)).toBe(expected);
  }

  it('任意二分切分：枚举每个码点边界，流式结果与一次性渲染一致', () => {
    const expected = renderChatMarkdown(DOC);
    const bounds = codePointBoundaries(DOC);
    for (let s = 1; s < bounds.length - 1; s++) {
      const i = bounds[s];
      simulateStreaming(`sw-${i}`, DOC, [DOC.slice(0, i), DOC.slice(i)], expected);
    }
  }, 120_000);

  it('逐字符到达：终态与一次性渲染一致', () => {
    const expected = renderChatMarkdown(DOC);
    simulateStreaming('char-by-char', DOC, chunksBySizes(DOC, [1]), expected);
  }, 120_000);

  it('变长 chunk 到达：终态与一次性渲染一致', () => {
    const expected = renderChatMarkdown(DOC);
    simulateStreaming('varied-small', DOC, chunksBySizes(DOC, [3, 5, 7, 11]), expected);
    simulateStreaming('varied-large', DOC, chunksBySizes(DOC, [50, 100, 200]), expected);
  }, 120_000);

  it('参考式链接（前向+后向引用）：任意二分切分终态一致', () => {
    const expected = renderChatMarkdown(REF_DOC);
    const bounds = codePointBoundaries(REF_DOC);
    for (let s = 1; s < bounds.length - 1; s++) {
      const i = bounds[s];
      simulateStreaming(`ref-${i}`, REF_DOC, [REF_DOC.slice(0, i), REF_DOC.slice(i)], expected);
    }
  }, 120_000);

  it('4 段切分组合：多次重渲染后仍一致', () => {
    const expected = renderChatMarkdown(SHORT_DOC);
    const bounds = codePointBoundaries(SHORT_DOC).filter((_, i) => i % 3 === 0);
    let n = 0;
    for (let a = 1; a < bounds.length - 2; a++) {
      for (let b = a + 1; b < bounds.length - 1; b++) {
        for (let c = b + 1; c < bounds.length; c++) {
          const i = bounds[a];
          const j = bounds[b];
          const k = bounds[c];
          simulateStreaming(
            `fw-${n++}`,
            SHORT_DOC,
            [SHORT_DOC.slice(0, i), SHORT_DOC.slice(i, j), SHORT_DOC.slice(j, k), SHORT_DOC.slice(k)],
            expected,
          );
        }
      }
    }
  }, 120_000);

  it('CRLF 换行：逐字符到达终态一致', () => {
    const crlf = SHORT_DOC.replace(/\n/g, '\r\n');
    const expected = renderChatMarkdown(crlf);
    simulateStreaming('crlf', crlf, chunksBySizes(crlf, [1]), expected);
  }, 120_000);

  it('切在 UTF-16 代理对中间：不崩溃且终态一致', () => {
    const full = '前文 🎉🎊 后文 **加粗**';
    const expected = renderChatMarkdown(full);
    const mid = full.indexOf('🎉') + 1; // 高/低位代理之间
    simulateStreaming('mb-split', full, [full.slice(0, mid), full.slice(mid)], expected);
  });

  it('重复渲染幂等：无新输入时输出逐位一致', () => {
    const content = DOC.slice(0, 200);
    const h1 = renderChatMarkdownForMessage('idem-1', content, true);
    const h2 = renderChatMarkdownForMessage('idem-1', content, true);
    const h3 = renderChatMarkdownForMessage('idem-1', content, true);
    expect(h2).toBe(h1);
    expect(h3).toBe(h1);
  });

  it('多条消息交错流式：per-message 状态互不影响', () => {
    const a = '# A\n\n内容甲 **粗体** `代码`';
    const b = '# B\n\n内容乙 [链接](https://b.example)';
    const ea = renderChatMarkdown(a);
    const eb = renderChatMarkdown(b);
    const stepA = chunksBySizes(a, [4]);
    const stepB = chunksBySizes(b, [6]);
    let accA = '';
    let accB = '';
    for (let k = 0; k < Math.max(stepA.length, stepB.length); k++) {
      if (stepA[k]) {
        accA += stepA[k];
        renderChatMarkdownForMessage('iso-a', accA, true);
      }
      if (stepB[k]) {
        accB += stepB[k];
        renderChatMarkdownForMessage('iso-b', accB, true);
      }
    }
    expect(renderChatMarkdownForMessage('iso-a', accA, false)).toBe(ea);
    expect(renderChatMarkdownForMessage('iso-b', accB, false)).toBe(eb);
  });
});

/**
 * 流式增量渲染（块级冻结，借鉴 xai-grok-markdown checkpoint 机制）。
 *
 * 冻结规则（CommonMark 风险清单）：
 * - 空行前的最近非空行是列表标记 → 不可冻结（松散列表会改变所有 item 的 <p> 包裹）
 * - 空行前的最近非空行是 ≥4 空格/tab 缩进 → 不可冻结（缩进代码块吸收空行）
 * - fence 内的空行 → 不产生边界；fence 闭合后的空行 → 可冻结
 * - EOF 处的空行 → 不冻结（块仍可能生长，对应 Grok 的代码块闭合才 checkpoint）
 * - 含链接引用定义（[label]: dest）→ 整条消息禁用冻结（前向引用可改变已冻结区渲染结果）
 * - 输入非 append-only → 整体重置（wholesale invalidation）
 */
describe('chatMessageMarkdown 流式增量渲染（块级冻结）', () => {
  beforeEach(() => clearChatMarkdownCache());

  describe('computeFrozenPrefixEnd 边界规则', () => {
    it('普通段落 + 空行 + 下一块：可冻结到块边界', () => {
      const content = '第一段。\n\n第二段。';
      expect(computeFrozenPrefixEnd(content)).toBe('第一段。\n\n'.length);
    });

    it('多个段落：冻结到最后一个已确认边界', () => {
      const content = '一。\n\n二。\n\n三。';
      expect(computeFrozenPrefixEnd(content)).toBe('一。\n\n二。\n\n'.length);
    });

    it('标题、引用、表格、水平线后的空行均可冻结', () => {
      const content = '# 标题\n\n> 引用\n\n| a |\n| - |\n| 1 |\n\n---\n\n下一段。';
      expect(computeFrozenPrefixEnd(content)).toBe(content.lastIndexOf('下一段。'));
    });

    it('列表项后空行不可冻结（松散列表风险）', () => {
      expect(computeFrozenPrefixEnd('- 项一\n\n- 项二')).toBe(0);
      expect(computeFrozenPrefixEnd('1. 项一\n\n- 项二')).toBe(0);
      expect(computeFrozenPrefixEnd('前置段落\n\n- 项一\n\n后续')).toBe('前置段落\n\n'.length);
    });

    it('缩进代码后空行不可冻结（空行会被吸收进代码块）', () => {
      expect(computeFrozenPrefixEnd('    code\n\n    more')).toBe(0);
      expect(computeFrozenPrefixEnd('\tcode\n\nnext')).toBe(0);
    });

    it('fence 内空行不产生边界；fence 闭合后空行可冻结', () => {
      expect(computeFrozenPrefixEnd('```js\nlet a = 1;\n\nlet b = 2;\n```\n\n后续')).toBe(
        '```js\nlet a = 1;\n\nlet b = 2;\n```\n\n'.length,
      );
      expect(computeFrozenPrefixEnd('```js\nlet a = 1;\n\nlet b = 2;')).toBe(0);
    });

    it('fence 未闭合时其前的边界仍可冻结', () => {
      const content = '前置段落。\n\n```js\ncode 生长中';
      expect(computeFrozenPrefixEnd(content)).toBe('前置段落。\n\n'.length);
    });

    it('EOF 处空行不冻结（块仍可能生长），但已确认边界保持有效', () => {
      expect(computeFrozenPrefixEnd('第一段。\n\n')).toBe(0);
      expect(computeFrozenPrefixEnd('第一段。\n\n第二段。\n\n')).toBe('第一段。\n\n'.length);
    });

    it('含链接引用定义时返回 0（禁用冻结）', () => {
      expect(computeFrozenPrefixEnd('[r1]: https://x.com\n\n段落 [a][r1]')).toBe(0);
      expect(computeFrozenPrefixEnd('段落 [a][r1]\n\n[r1]: https://x.com\n\n后续')).toBe(0);
    });

    it('fence 内的 [x]: y 不误判为引用定义', () => {
      const content = '```\n[r1]: https://x.com\n```\n\n后续段落';
      expect(computeFrozenPrefixEnd(content)).toBe(content.lastIndexOf('后续段落'));
    });

    it('CRLF 文档边界正确', () => {
      const content = '第一段。\r\n\r\n第二段。';
      expect(computeFrozenPrefixEnd(content)).toBe('第一段。\r\n\r\n'.length);
    });

    it('波浪线 fence 同样跟踪', () => {
      expect(computeFrozenPrefixEnd('~~~js\ncode\n\nmore\n~~~\n\n后续')).toBe('~~~js\ncode\n\nmore\n~~~\n\n'.length);
    });
  });

  describe('renderChatMarkdownParts 分段渲染', () => {
    it('frozenHtml + tailHtml 拼接与字符串接口一致', () => {
      const content = '# 标题\n\n第一段 **加粗**。\n\n第二段 `代码`。';
      const parts = renderChatMarkdownParts('p-1', content, true);
      expect(parts.frozenHtml + parts.tailHtml).toBe(renderChatMarkdownForMessage('p-1', content, true));
    });

    it('流式推进时冻结区前进、tail 缩短', () => {
      renderChatMarkdownParts('p-2', '第一段。', true);
      renderChatMarkdownParts('p-2', '第一段。\n\n第二段。', true);
      const parts = renderChatMarkdownParts('p-2', '第一段。\n\n第二段。\n\n第三段。', true);
      expect(parts.frozenHtml).toContain('第一段');
      expect(parts.frozenHtml).toContain('第二段');
      expect(parts.tailHtml).not.toContain('第一段');
      expect(parts.tailHtml).toContain('第三段');
    });

    it('追加 tail 时不重复渲染已冻结前缀（性能契约）', () => {
      const spy = vi.spyOn(MarkdownIt.prototype, 'render');
      try {
        renderChatMarkdownParts('p-3', '第一段。\n\n第二段。', true);
        const before = spy.mock.calls.length;
        // 追加内容使「第二段」冻结，只应渲染：新冻结段（第二段）+ tail（第三段）
        renderChatMarkdownParts('p-3', '第一段。\n\n第二段。\n\n第三段。', true);
        expect(spy.mock.calls.length - before).toBe(2);
        // 无新冻结点时只渲染 tail
        const before2 = spy.mock.calls.length;
        renderChatMarkdownParts('p-3', '第一段。\n\n第二段。\n\n第三段。追加', true);
        expect(spy.mock.calls.length - before2).toBe(1);
      } finally {
        spy.mockRestore();
      }
    });

    it('无新输入时不产生任何渲染（幂等）', () => {
      const spy = vi.spyOn(MarkdownIt.prototype, 'render');
      try {
        renderChatMarkdownParts('p-4', '第一段。\n\n第二段。', true);
        const before = spy.mock.calls.length;
        const a = renderChatMarkdownParts('p-4', '第一段。\n\n第二段。', true);
        const b = renderChatMarkdownParts('p-4', '第一段。\n\n第二段。', true);
        expect(spy.mock.calls.length - before).toBe(0);
        expect(b).toEqual(a);
      } finally {
        spy.mockRestore();
      }
    });

    it('内容回退（非 append-only）触发整体重置且结果正确', () => {
      renderChatMarkdownParts('p-5', '旧内容。\n\n旧第二段。', true);
      const fresh = '全新内容。';
      const parts = renderChatMarkdownParts('p-5', fresh, true);
      expect(parts.frozenHtml + parts.tailHtml).toBe(renderChatMarkdown(fresh));
    });

    it('含链接引用定义时禁用冻结：tail 为全量', () => {
      const content = '[r1]: https://x.com\n\n段落 [a][r1]。\n\n后续。';
      const parts = renderChatMarkdownParts('p-6', content, true);
      expect(parts.frozenHtml).toBe('');
      expect(parts.tailHtml).toBe(renderChatMarkdown(content));
    });

    it('非流式（streaming=false）返回 frozen 为空、tail 为全量渲染', () => {
      const content = '第一段。\n\n第二段。';
      const parts = renderChatMarkdownParts('p-7', content, false);
      expect(parts.frozenHtml).toBe('');
      expect(parts.tailHtml).toBe(renderChatMarkdown(content));
    });

    it('字符串接口在流式各阶段与全量渲染一致', () => {
      const content = '前置。\n\n- 松一\n\n- 松二\n\n```js\ncode\n```\n\n结尾。';
      let acc = '';
      for (const ch of content) {
        acc += ch;
        expect(renderChatMarkdownForMessage('p-8', acc, true)).toBe(renderChatMarkdown(acc));
      }
      expect(renderChatMarkdownForMessage('p-8', acc, false)).toBe(renderChatMarkdown(acc));
    }, 120_000);

    it('frozenSegments 拼接等于 frozenHtml，且冻结推进时追加不回改', () => {
      renderChatMarkdownParts('p-9', '第一段。\n\n第二段。', true);
      const grown = renderChatMarkdownParts('p-9', '第一段。\n\n第二段。\n\n第三段。', true);
      expect(grown.frozenSegments.join('')).toBe(grown.frozenHtml);
      expect(grown.frozenSegments.length).toBe(2);
      expect(grown.frozenSegments[0]).toBe(renderChatMarkdown('第一段。\n\n'));
      expect(grown.frozenSegments[1]).toBe(renderChatMarkdown('第二段。\n\n'));
    });

    it('整体失效时 frozenEpoch 递增且分段被替换', () => {
      const before = renderChatMarkdownParts('p-10', '旧内容。\n\n旧第二段。', true);
      expect(before.frozenEpoch).toBe(0);
      expect(before.frozenSegments.length).toBe(1);
      const after = renderChatMarkdownParts('p-10', '全新内容。\n\n第二段。', true);
      expect(after.frozenEpoch).toBe(1);
      expect(after.frozenSegments.join('')).toBe(renderChatMarkdown('全新内容。\n\n'));
      // 流式途中出现链接引用定义 → 边界回退，epoch 再次递增
      const withRef = renderChatMarkdownParts('p-10', '全新内容。\n\n第二段。\n\n[r1]: https://x.com\n\n尾。', true);
      expect(withRef.frozenEpoch).toBe(2);
      expect(withRef.frozenSegments).toEqual([]);
      expect(withRef.frozenHtml).toBe('');
    });

    it('finish（streaming=false）走 mdCache：重复渲染不重复 parse', () => {
      const spy = vi.spyOn(MarkdownIt.prototype, 'render');
      try {
        renderChatMarkdownParts('p-11', '第一段。\n\n第二段。', false);
        const before = spy.mock.calls.length;
        renderChatMarkdownParts('p-11', '第一段。\n\n第二段。', false);
        expect(spy.mock.calls.length - before).toBe(0);
      } finally {
        spy.mockRestore();
      }
    });

    it('finish 兜底：流式结束后全量渲染与增量拼接逐字节一致', () => {
      const content = '# 标题\n\n第一段 **加粗**。\n\n```js\nconst x = 1;\n```\n\n结尾。';
      let streamed = renderChatMarkdownParts('p-12', '', true);
      for (let i = 1; i <= content.length; i++) {
        streamed = renderChatMarkdownParts('p-12', content.slice(0, i), true);
      }
      const finished = renderChatMarkdownParts('p-12', content, false);
      expect(finished.frozenHtml).toBe('');
      expect(finished.frozenSegments).toEqual([]);
      expect(finished.frozenEpoch).toBe(0);
      // finish 全量渲染 === 流式最终拼接 === 一次性全量渲染
      expect(finished.tailHtml).toBe(streamed.frozenHtml + streamed.tailHtml);
      expect(finished.tailHtml).toBe(renderChatMarkdown(content));
    });

    it('finish 后流式状态被清理：同 id 再流式从零开始', () => {
      // 流式途中触发整体失效（链接引用定义回溯）→ epoch 递增
      renderChatMarkdownParts('p-13', '旧内容。\n\n第二段。', true);
      const duringStream = renderChatMarkdownParts('p-13', '旧内容。\n\n第二段。\n\n[r1]: https://x.com\n\n尾。', true);
      expect(duringStream.frozenEpoch).toBeGreaterThan(0);
      // finish → 增量状态销毁
      renderChatMarkdownParts('p-13', '旧内容。\n\n第二段。\n\n[r1]: https://x.com\n\n尾。', false);
      // 同 id 再次流式：fresh state，epoch 从 0 开始
      const restarted = renderChatMarkdownParts('p-13', '新内容。\n\n第二段。', true);
      expect(restarted.frozenEpoch).toBe(0);
      expect(restarted.frozenHtml + restarted.tailHtml).toBe(renderChatMarkdown('新内容。\n\n第二段。'));
    });
  });
});
