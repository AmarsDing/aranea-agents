/**
 * useConversationTimeline — 将 AgentBlock[] 转换为 ConversationTurn[]
 *
 * 基于"活动时间线"模型，每个 Turn = 用户消息 + Agent 工作过程。
 * 内部复用 useAgentBlocks 的数据构建逻辑，将 AgentBlock 转换为
 * ConversationTurn + Activity 模型。
 *
 * 设计文档：docs/reports/2026-06-12-proposal-chat-activity-timeline-redesign.md
 */

import { computed, type ComputedRef } from 'vue';
import type { Message } from '../types';
import type { Envelope } from '../../../realtime/envelope';
import type { AgentBlock, TimelineEntry } from '../agentTreeTypes';
import type {
  ConversationTurn,
  AgentWorkProcess,
  Activity,
  ThinkActivity,
  ActActivity,
  SayActivity,
  DelegateActivity,
  NoticeActivity,
  ToolActivity,
} from '../activityTimelineTypes';
import { useAgentBlocks } from './useAgentBlocks';

export function useConversationTimeline(deps: {
  messages: ComputedRef<Message[]>;
  isTeamSession?: boolean;
  plannerKind?: ComputedRef<string>;
  progressEnvelopes?: ComputedRef<readonly Envelope[]>;
}) {
  const { agentBlocks } = useAgentBlocks(deps);

  /**
   * 将 AgentBlock[] 转换为 ConversationTurn[]
   *
   * 每个 AgentBlock 对应一个 ConversationTurn：
   * - AgentBlock.task → ConversationTurn.userMessage（从 messages 中找）
   * - AgentBlock → AgentWorkProcess（字段映射 + timeline→activities 转换）
   */
  const conversationTurns = computed((): ConversationTurn[] => {
    const blocks = agentBlocks.value;
    if (blocks.length === 0) return [];

    const allMessages = deps.messages.value;
    return blocks.map((block) => convertToConversationTurn(block, allMessages));
  });

  return {
    conversationTurns,
    /** 保留 agentBlocks 引用，供渐进迁移使用 */
    agentBlocks,
  };
}

// ── Conversion helpers ──

function convertToConversationTurn(block: AgentBlock, allMessages: Message[]): ConversationTurn {
  // 找到对应的 user message
  const userMessage = findUserMessage(block, allMessages);

  return {
    id: block.id,
    userMessage,
    agentWork: convertToAgentWorkProcess(block),
  };
}

function findUserMessage(block: AgentBlock, allMessages: Message[]): Message {
  // AgentBlock.task 存储了用户消息的 content_markdown
  // AgentBlock.startedAt 存储了用户消息的 created_at
  // 尝试通过时间戳和内容匹配
  const match = allMessages.find(
    (m) =>
      m.role === 'user' &&
      m.content_markdown === block.task &&
      m.created_at === block.startedAt,
  );
  if (match) return match;

  // Fallback: 找到时间戳最接近的 user 消息
  const blockStart = block.startedAt ? new Date(block.startedAt).getTime() : 0;
  let closest: Message | null = null;
  let minDiff = Infinity;
  for (const m of allMessages) {
    if (m.role !== 'user') continue;
    const msgTime = new Date(m.created_at).getTime();
    const diff = Math.abs(msgTime - blockStart);
    if (diff < minDiff) {
      minDiff = diff;
      closest = m;
    }
  }

  // 最后 fallback：构造一个虚拟 user message
  if (!closest) {
    return {
      id: `${block.id}-user`,
      role: 'user',
      content_markdown: block.task || '',
      created_at: block.startedAt,
      status: 'done',
    } as Message;
  }

  return closest;
}

function convertToAgentWorkProcess(block: AgentBlock): AgentWorkProcess {
  return {
    agentKey: block.agentKey,
    agentName: block.agentName,
    agentIcon: block.agentIcon,
    agentColor: block.agentColor,
    status: mapAgentStatus(block.status),
    durationMs: block.durationMs,
    activities: block.timeline.map(convertTimelineEntry),
    // Team panel 暂不构建，Phase 3 实现
    task: block.task,
    result: block.result,
    hasPartialFailure: block.hasPartialFailure,
    plan: block.plan,
    teamStatus: block.teamStatus,
    progressSections: block.progressSections,
    startedAt: block.startedAt,
    finishedAt: block.finishedAt,
  };
}

/**
 * 将 AgentBlockStatus（6 种）映射为 AgentWorkProcess.status（3 种）
 * - tool_running / tool_blocked → running
 * - partial_failure → completed
 * - running / completed / failed → 保持不变
 */
function mapAgentStatus(
  status: AgentBlock['status'],
): AgentWorkProcess['status'] {
  switch (status) {
    case 'tool_running':
    case 'tool_blocked':
      return 'running';
    case 'partial_failure':
      return 'completed';
    default:
      return status;
  }
}

/** 将 TimelineEntry 转换为 Activity */
function convertTimelineEntry(entry: TimelineEntry): Activity {
  switch (entry.kind) {
    case 'thinking':
      return convertThinking(entry.section, entry.sortKey);
    case 'tool':
      return convertTool(entry.section);
    case 'reply':
      return convertReply(entry.section);
    case 'subagent':
      return convertSubagent(entry.block);
    case 'notice':
      return convertNotice(entry.section);
  }
}

function convertThinking(
  section: TimelineEntry extends { kind: 'thinking' } ? TimelineEntry['section'] : never,
  _sortKey: number,
): ThinkActivity {
  // TimelineEntry 的 thinking section 类型
  const s = section as { id: string; content: string; durationMs: number; collapsed: boolean; streaming: boolean };
  return {
    kind: 'think',
    id: s.id,
    content: s.content,
    collapsed: s.collapsed,
    streaming: s.streaming,
    durationMs: s.durationMs ?? null,
  };
}

function convertTool(
  section: TimelineEntry extends { kind: 'tool' } ? TimelineEntry['section'] : never,
): ActActivity {
  const s = section as {
    id: string; toolName: string; toolLabel: string;
    status: string; durationMs: number | null;
    arguments: string | null; result: string | null;
    error: string | null; iconKey?: string; isLongRunning?: boolean;
  };
  const tool: ToolActivity = {
    toolName: s.toolName,
    toolLabel: s.toolLabel,
    status: s.status as ToolActivity['status'],
    durationMs: s.durationMs,
    arguments: s.arguments,
    result: s.result,
    error: s.error,
    iconKey: s.iconKey,
    isLongRunning: s.isLongRunning,
  };
  return {
    kind: 'act',
    id: s.id,
    tool,
  };
}

function convertReply(
  section: TimelineEntry extends { kind: 'reply' } ? TimelineEntry['section'] : never,
): SayActivity {
  const s = section as { id: string; content: string; durationMs: number | null; streaming: boolean };
  return {
    kind: 'say',
    id: s.id,
    content: s.content,
    // isFinal 在构建时无法从单条 reply 判定，由渲染层根据位置推断
    // 后续 Phase 4 流式体验中会实现动态 isFinal
    isFinal: false,
    streaming: s.streaming,
    variant: 'default',
    durationMs: s.durationMs,
  };
}

function convertSubagent(block: AgentBlock): DelegateActivity {
  return {
    kind: 'delegate',
    id: block.id,
    subAgent: convertToAgentWorkProcess(block),
  };
}

function convertNotice(
  section: TimelineEntry extends { kind: 'notice' } ? TimelineEntry['section'] : never,
): NoticeActivity {
  const s = section as { id: string; type: 'degradation' | 'info'; message: string };
  return {
    kind: 'notice',
    id: s.id,
    type: s.type,
    message: s.message,
  };
}
