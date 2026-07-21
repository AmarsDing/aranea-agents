import type { Directive } from 'vue';
import type { ChatMarkdownParts } from './chatMessageMarkdown';

/**
 * v-segmented-markdown：流式 Markdown 的 DOM 分段渲染（借鉴 xai-grok-markdown
 * 的 checkpoint 冻结机制）。
 *
 * 渲染策略（配合 renderChatMarkdownParts 的输出契约）：
 * - 冻结段（frozenSegments）解析后直接作为容器的子节点**追加**，节点一旦插入
 *   永不触碰——用户文本选择、代码块展开等交互状态在流式期间完整保留；
 * - 尾部（tailHtml）整体替换，新冻结段插入到 tail 节点之前；
 * - frozenEpoch 变化（整体失效）时全量重建；
 * - 所有块节点都是容器的直接子节点，与单容器 v-html 的 DOM 结构完全一致，
 *   chat-message-prose 的 :first-child / :last-child / 相邻兄弟选择器行为不变。
 *
 * 用法：
 *   <div v-segmented-markdown="parts" class="chat-message-prose"></div>
 *   const parts = computed(() => renderChatMarkdownParts(id, content, streaming));
 */

interface SegDomState {
  epoch: number;
  segCount: number;
  tailHtml: string;
  tailNodes: Node[];
}

const domStates = new WeakMap<HTMLElement, SegDomState>();

function parseHtmlNodes(html: string): Node[] {
  const tpl = document.createElement('template');
  tpl.innerHTML = html;
  return Array.from(tpl.content.childNodes);
}

function removeNodes(nodes: Node[]): void {
  for (const n of nodes) n.parentNode?.removeChild(n);
}

/** 增量对齐 DOM 到 parts；幂等，可安全重复调用 */
export function reconcileSegmentedMarkdown(el: HTMLElement, parts: ChatMarkdownParts): void {
  let st = domStates.get(el);
  if (!st || st.epoch !== parts.frozenEpoch || st.segCount > parts.frozenSegments.length) {
    // 整体失效（或状态不一致）：全量重建
    el.textContent = '';
    st = { epoch: parts.frozenEpoch, segCount: 0, tailHtml: '', tailNodes: [] };
    domStates.set(el, st);
  }

  // 追加新冻结段（插入到 tail 节点之前，保持文档顺序）
  const segs = parts.frozenSegments;
  if (st.segCount < segs.length) {
    const before = st.tailNodes[0] ?? null;
    for (let i = st.segCount; i < segs.length; i++) {
      for (const node of parseHtmlNodes(segs[i]!)) {
        el.insertBefore(node, before);
      }
    }
    st.segCount = segs.length;
  }

  // 替换 tail（每次输入变化的唯一重渲染区）
  if (parts.tailHtml !== st.tailHtml) {
    removeNodes(st.tailNodes);
    st.tailNodes = parseHtmlNodes(parts.tailHtml);
    for (const n of st.tailNodes) el.appendChild(n);
    st.tailHtml = parts.tailHtml;
  }
}

export const vSegmentedMarkdown: Directive<HTMLElement, ChatMarkdownParts> = {
  mounted(el, binding) {
    reconcileSegmentedMarkdown(el, binding.value);
  },
  updated(el, binding) {
    reconcileSegmentedMarkdown(el, binding.value);
  },
};
