/**
 * useConversationTimeline — 直接从原始消息构建 ConversationTurn[]
 *
 * 不再包装 useAgentBlocks，直接按 role=user 边界划分 Turn，
 * 在每个 Turn 内按消息时间顺序构建 Activity（Think / Act / Say）。
 *
 * 设计原则：
 * 1. 按 role=user 边界划分 Turn（堆栈模型，红线 #14）
 * 2. assistant 消息的 reasoning_markdown → ThinkActivity
 * 3. assistant 消息的 content_markdown（非 reasoning 部分）→ SayActivity
 * 4. tool 消息 → ActActivity
 * 5. 同一 Turn 内多个 assistant 消息（被 tool 分隔）各自独立生成 Think+Say
 * 6. 后端 fallback 去重：content_markdown === reasoning_markdown 时只创建 SayActivity
 * 7. isFinal：Turn 内最后一个 SayActivity 的 isFinal=true
 */

import { computed, type ComputedRef } from 'vue';
import type { Message, ToolUseEvent } from '../types';
import type { Envelope } from '../../../realtime/envelope';
import type {
  ConversationTurn,
  AgentWorkProcess,
  Activity,
  ActActivity,
  SayActivity,
  ToolActivity,
} from '../activityTimelineTypes';
import type { AgentBlock } from '../agentTreeTypes';
import { agentColorFromKey, ROOT_AGENT_KEY } from '../agentTreeTypes';
import { toolEventFromMessage } from '../envelopeToolCall';
import { resolveAssistantPresentation } from '../messagePlannerPresentation';
import { resolveDisplayLabel } from '../activityPresentation';
import { isTeamMemberOrigin, ensureOrigin } from '../messageOrigin';
import { canonicalToolStatus } from '../lib/statusMap';

// ── UserTurn (internal) ──

interface UserTurn {
  userMessage: Message | null;
  messages: Message[];
}

// ── Main composable ──

export function useConversationTimeline(deps: {
  messages: ComputedRef<Message[]>;
  /** TECH-DEBT: Phase 3 — Team 模式 DelegateActivity 构建时使用 */
  isTeamSession?: boolean;
  plannerKind?: ComputedRef<string>;
  /** TECH-DEBT: Phase 3 — Team 进度面板构建时使用 */
  progressEnvelopes?: ComputedRef<readonly Envelope[]>;
}) {
  const conversationTurns = computed((): ConversationTurn[] => {
    const allMessages = deps.messages.value;
    if (allMessages.length === 0) return [];

    const plannerKind = deps.plannerKind?.value ?? '';
    const ensured = allMessages.map((m) => ensureOrigin(m));

    // 1. 按 role=user 边界划分 Turn
    const turns = findUserTurns(ensured);

    // 2. 为每个 Turn 构建 ConversationTurn
    return turns.map((turn) => buildConversationTurn(turn, plannerKind));
  });

  return {
    conversationTurns,
    /** TECH-DEBT: Phase 3 迁移完成后移除 — 始终返回空数组 */
    agentBlocks: computed((): AgentBlock[] => []),
  };
}

// ── findUserTurns: 按 role=user 边界划分 ──

function findUserTurns(messages: Message[]): UserTurn[] {
  if (messages.length === 0) return [];
  const turns: UserTurn[] = [];
  let current: UserTurn = { userMessage: null, messages: [] };

  for (const msg of messages) {
    if (msg.role === 'user') {
      if (current.messages.length > 0 || current.userMessage) {
        turns.push(current);
      }
      current = { userMessage: msg, messages: [msg] };
    } else {
      current.messages.push(msg);
    }
  }

  if (current.messages.length > 0 || current.userMessage) {
    turns.push(current);
  }

  return turns;
}

// ── buildConversationTurn: 核心逻辑 ──

function buildConversationTurn(turn: UserTurn, plannerKind: string): ConversationTurn {
  const msgs = turn.messages;
  const userMessage = turn.userMessage || msgs.find((m) => m.role === 'user');

  // 找到第一个 assistant 消息来确定 agent 信息
  const firstAssistant = msgs.find((m) => m.role === 'assistant' && !isTeamMemberOrigin(m.origin));
  const agentKey = firstAssistant?.agent_ref?.agent_key || ROOT_AGENT_KEY;
  const agentName = firstAssistant?.agent_ref?.name || '精灵助手';
  const agentIcon = firstAssistant?.agent_ref?.icon || '精';

  // 按时间顺序遍历消息，构建 Activity 列表
  const activities: Activity[] = [];
  let sortCounter = 0;
  let lastSayIndex = -1; // 追踪最后一个 SayActivity 的索引，用于 isFinal

  for (let i = 0; i < msgs.length; i++) {
    const msg = msgs[i];
    if (msg.role === 'user') continue;

    // 1. Tool 消息 → ActActivity
    const toolEv = toolEventFromMessage(msg);
    if (toolEv) {
      activities.push(buildActActivity(toolEv, sortCounter++));
      continue;
    }

    // 2. Team member 消息 → 暂时跳过
    if (isTeamMemberOrigin(msg.origin)) {
      // TODO: Phase 3 - 构建 DelegateActivity
      continue;
    }

    // 3. Assistant 消息 → ThinkActivity + SayActivity
    if (msg.role === 'assistant') {
      const presentation = resolveAssistantPresentation(plannerKind, msg);
      const reasoning = presentation.reasoning?.trim() || '';
      const bodyMarkdown = presentation.bodyMarkdown?.trim() || '';

      // ReAct 模式：每个 thinking-kind step 生成一个 ThinkActivity
      if (presentation.reactSteps?.steps?.length) {
        for (const step of presentation.reactSteps.steps) {
          const isThinking = step.kind === 'planning' || step.kind === 'reasoning' || step.kind === 'replanning';
          if (isThinking && step.body?.trim()) {
            activities.push({
              kind: 'think',
              id: `think-${msg.id}-${step.kind}-${sortCounter}`,
              content: step.body,
              label: step.kind,
              collapsed: true,
              streaming: msg.status === 'streaming',
              durationMs: null,
            });
            sortCounter++;
          }
        }
        // ReAct final answer → SayActivity
        if (presentation.reactSteps.hasExplicitFinalAnswer && presentation.reactSteps.finalAnswer?.trim()) {
          activities.push({
            kind: 'say',
            id: `say-${msg.id}`,
            content: presentation.reactSteps.finalAnswer,
            isFinal: false, // 后面统一设置
            streaming: msg.status === 'streaming',
            variant: 'default',
            durationMs: null,
          });
          lastSayIndex = activities.length - 1;
          sortCounter++;
        }
      } else {
        // 非 ReAct 模式
        // 判断是否为后端 fallback：content_markdown 和 reasoning_markdown 内容相同
        // 后端 DisplayMarkdownFromStream 在 LLM 只有 reasoning 没有 text 时，
        // 会将 reasoning 作为 content_markdown 持久化，导致两者内容相同。
        // 此时应该只创建 SayActivity（这是 LLM 的实际回复），不创建 ThinkActivity。
        const isBackendFallback = reasoning && bodyMarkdown && bodyMarkdown === reasoning;

        if (reasoning && !isBackendFallback) {
          // reasoning 有独立内容，且 content_markdown 不同 → 创建 ThinkActivity
          activities.push({
            kind: 'think',
            id: `think-${msg.id}`,
            content: reasoning,
            collapsed: true,
            streaming: msg.status === 'streaming',
            durationMs: null,
          });
          sortCounter++;
        }

        if (bodyMarkdown) {
          // content_markdown 有内容 → 创建 SayActivity
          // （后端 fallback 时 bodyMarkdown === reasoning，此时只创建 SayActivity）
          activities.push({
            kind: 'say',
            id: `say-${msg.id}`,
            content: bodyMarkdown,
            isFinal: false,
            streaming: msg.status === 'streaming',
            variant: presentation.mode === 'a2ui' ? 'a2ui' : 'default',
            durationMs: null,
          });
          lastSayIndex = activities.length - 1;
          sortCounter++;
        }
      }
    }
  }

  // 设置 isFinal：最后一个 SayActivity 的 isFinal = true
  if (lastSayIndex >= 0) {
    const lastSay = activities[lastSayIndex];
    if (lastSay.kind === 'say') {
      (lastSay as SayActivity).isFinal = true;
    }
  }

  // 计算状态
  const isCompleted = !msgs.some(
    (m) => m.status === 'streaming' || m.status === 'tool_running' || m.status === 'tool_blocked',
  );
  const hasFailedTool = msgs.some((m) => {
    const ev = toolEventFromMessage(m);
    return ev?.status === 'failed';
  });
  const status: AgentWorkProcess['status'] = isCompleted ? (hasFailedTool ? 'failed' : 'completed') : 'running';

  // 计算时长
  const startTs = userMessage?.created_at ? new Date(userMessage.created_at).getTime() : 0;
  const lastMsg = msgs[msgs.length - 1];
  const endTs = lastMsg?.created_at ? new Date(lastMsg.created_at).getTime() : 0;
  const durationMs = startTs && endTs ? Math.max(0, endTs - startTs) : null;

  const agentWork: AgentWorkProcess = {
    agentKey,
    agentName,
    agentIcon,
    agentColor: agentColorFromKey(agentKey),
    status,
    durationMs,
    activities,
    task: userMessage?.content_markdown || null,
    result: lastSayIndex >= 0 && activities[lastSayIndex].kind === 'say'
      ? (activities[lastSayIndex] as SayActivity).content
      : null,
    hasPartialFailure: false, // TECH-DEBT: 未计算 hasPartialFailure
    plan: null,
    teamStatus: null,
    progressSections: [],
    startedAt: userMessage?.created_at || firstAssistant?.created_at || '',
    finishedAt: isCompleted ? lastMsg?.created_at || '' : null,
  };

  return {
    id: `turn-${userMessage?.id || 'no-user'}`,
    userMessage: userMessage ?? null,
    agentWork,
  };
}

// ── buildActActivity: 从 ToolUseEvent 构建 ActActivity ──

function buildActActivity(toolEv: ToolUseEvent, sortKey: number): ActActivity {
  const status = canonicalToolStatus(toolEv.status);
  const tool: ToolActivity = {
    toolName: toolEv.tool_name,
    toolLabel: resolveDisplayLabel(toolEv),
    status,
    durationMs: toolEv.duration_ms ?? null,
    arguments: toolEv.arguments != null ? JSON.stringify(toolEv.arguments, null, 2) : null,
    result: toolEv.result != null ? JSON.stringify(toolEv.result, null, 2) : toolEv.error || null,
    error: toolEv.error || null,
    iconKey: toolEv.icon_key,
    isLongRunning: toolEv.is_long_running === true,
  };
  return {
    kind: 'act',
    id: `act-${toolEv.id || sortKey}`,
    tool,
  };
}
