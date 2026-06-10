/**
 * 紧凑时间线节点构建（non-ReAct 模式）
 *
 * 设计目标：替换原 `interleavedRounds` 的 1:1 配对渲染。
 * - 不切分段落（消除 `split(/\n{2,}/)` 的脆弱性）
 * - 不配对 thinking/reply（消除无因果信号的位置配对）
 * - 不均分工具（消除 `Math.ceil(tools/rounds)` 的失真）
 *
 * 节点顺序如实反映实际事件流：thinking → tools → reply
 *
 * @see docs/reports/2026-06-10-proposal-chat-compact-timeline.md
 */

import type { ToolUseEvent } from './types';

/** 紧凑时间线节点。 */
export type CompactNode =
  | { kind: 'thinking'; text: string; messageId: string }
  | { kind: 'tool'; event: ToolUseEvent }
  | { kind: 'reply'; text: string; messageId: string; streaming: boolean; status: 'ok' | 'failed' | 'cancelled' | 'streaming' };

/** Reply 节点类型（外部消费者可导入以约束函数签名） */
export type ReplyNode = Extract<CompactNode, { kind: 'reply' }>;

/** Reply 节点 status 字段类型 */
export type ReplyStatus = ReplyNode['status'];

export type BuildCompactNodesArgs = {
  /** 累积的思考文本（来自 `message.reasoning_markdown`） */
  reasoning: string;
  /** 累积的回复文本（来自 `message.content_markdown`） */
  bodyMarkdown: string;
  /** 工具事件（已过滤 ReAct 链入的工具） */
  toolEvents: ToolUseEvent[];
  /** 关联的 message id（用于稳定 key） */
  messageId: string;
  /** 是否正在流式 */
  isStreaming: boolean;
  /** 消息状态（决定 reply 节点 status） */
  messageStatus?: string;
};

/**
 * 构造紧凑时间线节点。
 *
 * 规则：
 * 1. reasoning 非空 → 1 个 thinking 节点（不切分）
 * 2. 每个 tool → 1 个 tool 节点（保持原顺序）
 * 3. body 非空 → 1 个 reply 节点（不切分）
 *
 * 边界：
 * - 全为空 → []
 * - reasoning + body 为空 + 有 tool → [tool, ...]
 * - reply 在 thinking/tools 之后（符合用户期待"先思考再回复"）
 */
export function buildCompactNodes(args: BuildCompactNodesArgs): CompactNode[] {
  const { reasoning, bodyMarkdown, toolEvents, messageId, isStreaming, messageStatus } = args;
  const nodes: CompactNode[] = [];

  // 1) 思考（整段，不切分）
  const trimmedReasoning = reasoning?.trim() ?? '';
  if (trimmedReasoning) {
    nodes.push({ kind: 'thinking', text: trimmedReasoning, messageId });
  }

  // 2) 工具（保持原顺序）
  for (const event of toolEvents) {
    nodes.push({ kind: 'tool', event });
  }

  // 3) 回复（整段，不切分）
  const trimmedBody = bodyMarkdown?.trim() ?? '';
  if (trimmedBody) {
    nodes.push({
      kind: 'reply',
      text: trimmedBody,
      messageId,
      streaming: isStreaming,
      status: deriveReplyStatus(messageStatus, isStreaming),
    });
  }

  return nodes;
}

function deriveReplyStatus(
  messageStatus: string | undefined,
  isStreaming: boolean,
): 'ok' | 'failed' | 'cancelled' | 'streaming' {
  if (isStreaming) return 'streaming';
  if (messageStatus === 'failed' || messageStatus === 'tool_failed') return 'failed';
  if (messageStatus === 'cancelled' || messageStatus === 'tool_cancelled') return 'cancelled';
  return 'ok';
}

/**
 * 生成稳定 Vue key。
 *
 * 用 `${messageId}-${kind}` 避免流式到达时整列表重渲染。
 */
export function compactNodeKey(node: CompactNode): string {
  switch (node.kind) {
    case 'thinking':
      return `${node.messageId}-thinking`;
    case 'tool':
      return `${node.messageId}-tool-${node.event.id}`;
    case 'reply':
      return `${node.messageId}-reply`;
  }
}
