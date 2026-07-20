import { describe, it, expect, beforeEach } from 'vitest';
import { reconcileSegmentedMarkdown } from '../vSegmentedMarkdown';
import { renderChatMarkdownParts, clearChatMarkdownCache } from '../chatMessageMarkdown';

describe('vSegmentedMarkdown DOM 分段渲染', () => {
  beforeEach(() => clearChatMarkdownCache());

  function host(): HTMLElement {
    return document.createElement('div');
  }

  it('初始渲染：tail 节点作为容器直接子节点', () => {
    const el = host();
    reconcileSegmentedMarkdown(el, renderChatMarkdownParts('d-1', '第一段。', true));
    expect(el.innerHTML).toContain('<p>第一段。</p>');
    expect(el.children.length).toBe(1);
  });

  it('冻结推进：新段插入 tail 之前，已冻结节点身份不变', () => {
    const el = host();
    reconcileSegmentedMarkdown(el, renderChatMarkdownParts('d-2', '第一段。\n\n第二段。', true));
    const frozenP = el.querySelector('p');
    expect(frozenP?.textContent).toBe('第一段。');

    reconcileSegmentedMarkdown(el, renderChatMarkdownParts('d-2', '第一段。\n\n第二段。\n\n第三段。', true));

    expect(el.querySelector('p')).toBe(frozenP); // 冻结节点未被替换
    const ps = el.querySelectorAll('p');
    expect(ps.length).toBe(3);
    expect(ps[0]!.textContent).toBe('第一段。');
    expect(ps[1]!.textContent).toBe('第二段。');
    expect(ps[2]!.textContent).toBe('第三段。');
  });

  it('tail 追加：仅尾部重渲染，冻结与结构不变', () => {
    const el = host();
    reconcileSegmentedMarkdown(el, renderChatMarkdownParts('d-3', '第一段。\n\n第二段。', true));
    const frozenP = el.querySelector('p');
    reconcileSegmentedMarkdown(el, renderChatMarkdownParts('d-3', '第一段。\n\n第二段。追加', true));
    expect(el.querySelector('p')).toBe(frozenP);
    expect(el.textContent).toContain('第二段。追加');
  });

  it('epoch 变化（整体失效）：全量重建，旧节点全部替换', () => {
    const el = host();
    reconcileSegmentedMarkdown(el, renderChatMarkdownParts('d-4', '旧内容。\n\n旧第二段。', true));
    const oldP = el.querySelector('p');
    reconcileSegmentedMarkdown(el, renderChatMarkdownParts('d-4', '全新内容。', true));
    expect(el.querySelector('p')).not.toBe(oldP);
    expect(el.textContent).toContain('全新内容。');
    expect(el.textContent).not.toContain('旧内容。');
  });

  it('幂等：相同 parts 重复 reconcile 不产生任何 DOM 变化', () => {
    const el = host();
    const parts = renderChatMarkdownParts('d-5', '第一段。\n\n第二段。', true);
    reconcileSegmentedMarkdown(el, parts);
    const html = el.innerHTML;
    const firstP = el.querySelector('p');
    reconcileSegmentedMarkdown(el, parts);
    reconcileSegmentedMarkdown(el, parts);
    expect(el.innerHTML).toBe(html);
    expect(el.querySelector('p')).toBe(firstP);
  });

  it('非流式（streaming=false）：frozen 为空，tail 为全量渲染', () => {
    const el = host();
    reconcileSegmentedMarkdown(el, renderChatMarkdownParts('d-6', '# 标题\n\n正文。', false));
    expect(el.querySelector('h1')?.textContent).toBe('标题');
    expect(el.textContent).toContain('正文。');
  });

  it('DOM 结构与全量 v-html 渲染一致（块节点均为容器直接子节点）', () => {
    const el = host();
    reconcileSegmentedMarkdown(
      el,
      renderChatMarkdownParts('d-7', '# 标题\n\n第一段。\n\n- 项一\n- 项二\n\n结尾。', true),
    );
    // h1 + p + ul + p 全部是一级子节点，无包裹层
    const tags = Array.from(el.children).map((c) => c.tagName.toLowerCase());
    expect(tags).toEqual(['h1', 'p', 'ul', 'p']);
  });

  it('finish 后全量替换：流式分段 DOM 被完整全量渲染替换', () => {
    const el = host();
    reconcileSegmentedMarkdown(el, renderChatMarkdownParts('d-8', '第一段。\n\n第二段。', true));
    const frozenP = el.querySelector('p');
    // finish：streaming=false → frozenSegments 为空 → 全量重建
    reconcileSegmentedMarkdown(el, renderChatMarkdownParts('d-8', '第一段。\n\n第二段。', false));
    expect(el.querySelector('p')).not.toBe(frozenP);
    expect(el.textContent).toContain('第一段。');
    expect(el.textContent).toContain('第二段。');
  });
});
