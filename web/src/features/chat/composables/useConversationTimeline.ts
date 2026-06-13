/**
 * useConversationTimeline — 直接从原始消息或 Activity 事件构建 ConversationTurn[]
 *
 * 双路径架构（Activity-First 过渡期）：
 * - AF 路径（优先）：当 activityTimelineActivities 非空时，从后端 Activity 事件
 *   构建 Turn，零推理，语义由后端直推。
 * - 消息推理路径（fallback）：无 AF 数据时，从原始消息按 role=user 边界划分 Turn，
 *   在每个 Turn 内按消息时间顺序推断 Activity（Think / Act / Say）。
 *
 * TECH-DEBT: 消息推理路径将在 Phase 3 完全移除，届时所有 Turn 均从 Activity 构建。
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
  ThinkActivity,
  ToolActivity,
  TeamPanel,
  TaskBoardSection,
  DagSection,
  TeamProgressSection,
  AgentProgress,
} from '../activityTimelineTypes';
import type { ActivityTreeNode } from '../activityTypes';
import type { AgentBlock, TaskBoardNodeData, ProgressSection } from '../agentTreeTypes';
import { agentColorFromKey, ROOT_AGENT_KEY } from '../agentTreeTypes';
import { activityToTaskBoardNode, activityToTimelineActivity } from './useActivityTimeline';
import { toolEventFromMessage } from '../envelopeToolCall';
import { resolveAssistantPresentation } from '../messagePlannerPresentation';
import { resolveDisplayLabel } from '../activityPresentation';
import { isTeamMemberOrigin, ensureOrigin } from '../messageOrigin';
import { canonicalToolStatus } from '../lib/statusMap';
import { isReasoningAsDisplay } from '../streamContentPatch';
import { mergeProgressEvents } from '../executionProgress';

// ── UserTurn (internal) ──

interface UserTurn {
  userMessage: Message | null;
  messages: Message[];
}

// ── Helpers ──

/** Build sorted ProgressSection[] from execution_progress envelopes, clamping startedAt to turn start. */
function buildProgressSections(
  progressByStep: Map<string, ProgressSection>,
  userMessage: Message | undefined,
): ProgressSection[] {
  if (progressByStep.size === 0) return [];
  const turnStartTs = userMessage?.created_at
    ? new Date(userMessage.created_at).getTime()
    : Date.now();
  const sections: ProgressSection[] = [];
  for (const section of progressByStep.values()) {
    // Clamp startedAt to turn start to avoid progress events appearing before the turn
    sections.push({ ...section, startedAt: Math.max(turnStartTs, section.startedAt) });
  }
  sections.sort((a, b) => a.startedAt - b.startedAt);
  return sections;
}

// ── Main composable ──

export function useConversationTimeline(deps: {
  messages: ComputedRef<Message[]>;
  /** TECH-DEBT: Phase 3 — Team 模式 DelegateActivity 构建时使用 */
  isTeamSession?: boolean;
  plannerKind?: ComputedRef<string>;
  /** TECH-DEBT: Phase 3 — Team 进度面板构建时使用 */
  progressEnvelopes?: ComputedRef<readonly Envelope[]>;
  /** AF-FE-06: Activity-First data source — when provided, skips message inference */
  activityTimelineActivities?: ComputedRef<readonly Activity[]>;
  /** AF-FE-06: Agent info from Activity data */
  activityAgentKey?: ComputedRef<string>;
  /** AF-FE-06: Root task content from Activity data */
  activityTaskContent?: ComputedRef<string | null>;
  /** AF-FE-06: Activity tree for building TeamPanel */
  activityTree?: ComputedRef<readonly ActivityTreeNode[]>;
  /** AF-FE-14: Raw Activity records (with turnId) for grouping by turn */
  activityRawRecords?: ComputedRef<readonly import('../activityTypes').Activity[]>;
}) {
  const conversationTurns = computed((): ConversationTurn[] => {
    const allMessages = deps.messages.value;
    if (allMessages.length === 0) return [];

    // Merge execution_progress envelopes into ProgressSection map (shared by both paths)
    const progressByStep = deps.progressEnvelopes?.value
      ? mergeProgressEvents(deps.progressEnvelopes.value)
      : new Map<string, ProgressSection>();

    // AF-FE-06 + AF-FE-14: When Activity-First data is available, build turns
    // from Activity events. Uses buildAllConversationTurnsFromActivities which
    // groups raw Activity records by turnId to build ALL turns from AF data —
    // eliminating message inference entirely. Turns without Activity data
    // (e.g., pre-AF sessions) fall back to message inference per-turn.
    //
    // U1 fix: Activate AF path when raw Activity records exist, even if
    // timelineActivities is empty. This handles the "first-byte wait" period
    // where activity_start(kind=task) has been emitted but no thinking/action
    // activities exist yet. The resulting ConversationTurn will have
    // status=running with empty activities, causing AgentWorkPanel to show
    // the "正在思考…" indicator immediately.
    const afActivities = deps.activityTimelineActivities?.value;
    const afTree = deps.activityTree?.value;
    const afRawRecords = deps.activityRawRecords?.value;
    const hasAfData = (afActivities && afActivities.length > 0) || (afRawRecords && afRawRecords.length > 0);
    if (hasAfData) {
      return buildAllConversationTurnsFromActivities(allMessages, afRawRecords ?? [], afActivities ?? [], {
        agentKey: deps.activityAgentKey?.value || '',
        taskContent: deps.activityTaskContent?.value || null,
        activityTree: afTree ? [...afTree] : [],
        progressByStep,
      });
    }

    // TECH-DEBT(AF-Phase3): 消息推理路径 — 将在 Activity-First 完全上线后移除。
    // 届时所有 Turn 均从后端 Activity 事件构建，不再需要前端推断。
    const plannerKind = deps.plannerKind?.value ?? '';
    const ensured = allMessages.map((m) => ensureOrigin(m));

    // 1. 按 role=user 边界划分 Turn
    const turns = findUserTurns(ensured);

    // 2. 为每个 Turn 构建 ConversationTurn
    return turns.map((turn) => buildConversationTurn(turn, plannerKind, progressByStep));
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

function buildConversationTurn(turn: UserTurn, plannerKind: string, progressByStep: Map<string, ProgressSection>): ConversationTurn {
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
        // 判断后端标记：reasoning_as_display 表示 content_markdown 是 reasoning 的 fallback
        // 此时应该创建 ThinkActivity（作为主要回复），不创建 SayActivity
        const hasReasoningAsDisplayFlag = isReasoningAsDisplay(msg);

        // 兼容旧数据：没有 reasoning_as_display 标记时，用内容比较判断
        const isBackendFallback = reasoning && bodyMarkdown && bodyMarkdown === reasoning;

        if (hasReasoningAsDisplayFlag || isBackendFallback) {
          // reasoning 是实际回复内容 → 创建 SayActivity 作为主要展示
          // 后端标记 reasoning_as_display 表示 content_markdown 是 reasoning 的 fallback，
          // 此时 reasoning 就是实际回复，应显示为 SayActivity（💬 回复样式）
          activities.push({
            kind: 'say',
            id: `say-${msg.id}`,
            content: reasoning || bodyMarkdown,
            isFinal: false, // 后面统一设置
            streaming: msg.status === 'streaming',
            variant: 'default',
            durationMs: null,
          });
          lastSayIndex = activities.length - 1;
          sortCounter++;
        } else {
          // reasoning 和 content 不同，各自独立
          if (reasoning) {
            activities.push({
              kind: 'think',
              id: `think-${msg.id}`,
              content: reasoning,
              // During streaming, keep reasoning expanded so user can see
              // the thinking process in real-time. Collapse after completion.
              collapsed: msg.status !== 'streaming',
              streaming: msg.status === 'streaming',
              durationMs: null,
            });
            sortCounter++;
          }

          if (bodyMarkdown) {
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

  // Build progressSections from execution_progress envelopes
  const progressSections = buildProgressSections(progressByStep, userMessage);

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
      : activities.length > 0 && activities[activities.length - 1].kind === 'think'
        ? (activities[activities.length - 1] as ThinkActivity).content
        : null,
    hasPartialFailure: false, // TECH-DEBT: 未计算 hasPartialFailure
    plan: null,
    teamStatus: null,
    progressSections,
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

// ── AF-FE-14: Build ALL ConversationTurns from raw Activity records ──

/**
 * Build ConversationTurns for ALL turns by grouping raw Activity records
 * (which contain turnId) by turn. This eliminates the need for message
 * inference on historical turns — every turn is built from Activity data.
 *
 * Fallback: if a user turn has no matching Activity records (e.g., pre-AF
 * sessions), it falls back to `buildConversationTurn` for that turn only.
 */
function buildAllConversationTurnsFromActivities(
  messages: Message[],
  rawRecords: readonly import('../activityTypes').Activity[],
  timelineActivities: readonly Activity[],
  opts: { agentKey: string; taskContent: string | null; activityTree: ActivityTreeNode[]; progressByStep: Map<string, ProgressSection> },
): ConversationTurn[] {
  const ensured = messages.map(ensureOrigin);
  const userTurns = findUserTurns(ensured);
  if (userTurns.length === 0) return [];

  // Group raw Activity records by turnId
  const activitiesByTurn = new Map<string, import('../activityTypes').Activity[]>();
  for (const record of rawRecords) {
    const tid = record.turnId;
    if (!tid) continue;
    const group = activitiesByTurn.get(tid);
    if (group) group.push(record);
    else activitiesByTurn.set(tid, [record]);
  }

  const result: ConversationTurn[] = [];

  for (const turn of userTurns) {
    const userMessage = turn.userMessage || turn.messages.find((m) => m.role === 'user');
    // Match turn by finding the user message's turn_id, or by matching
    // the first assistant message's turn_id in the turn's messages.
    const turnId = userMessage?.turn_id
      || turn.messages.find((m) => m.role === 'assistant')?.turn_id
      || '';

    const turnActivities = turnId ? activitiesByTurn.get(turnId) : undefined;

    if (turnActivities && turnActivities.length > 0) {
      // Build from Activity data — zero inference
      result.push(buildSingleTurnFromActivities(turn, turnActivities, opts));
    } else {
      // AF-Phase3: No Activity data for this turn (pre-AF session or API failure).
      // Build a minimal ConversationTurn instead of falling back to message
      // inference. The turn shows the user message and a "historical" label;
      // detailed content (reasoning, tool calls) is available via the
      // original messages but not reconstructed through inference.
      result.push(buildLegacyConversationTurn(turn, opts));
    }
  }

  return result;
}

/**
 * AF-Phase3: Build a minimal ConversationTurn for turns without Activity data.
 * Shows the user message and a "historical conversation" label with basic
 * agent info extracted from the message metadata. No message inference is
 * performed — reasoning, tool calls, and ReAct steps are not reconstructed.
 *
 * This replaces the previous fallback to `buildConversationTurn` which
 * performed 13-layer inference (resolveAssistantPresentation, isReasoningAsDisplay,
 * parseReactPlannerContent, etc.).
 */
function buildLegacyConversationTurn(
  turn: UserTurn,
  opts: { agentKey: string; taskContent: string | null; activityTree: ActivityTreeNode[]; progressByStep: Map<string, ProgressSection> },
): ConversationTurn {
  const userMessage = turn.userMessage || turn.messages.find((m) => m.role === 'user');
  const firstAssistant = turn.messages.find((m) => m.role === 'assistant' && !isTeamMemberOrigin(m.origin));
  const lastMsg = turn.messages[turn.messages.length - 1];

  const agentKey = opts.agentKey || firstAssistant?.agent_ref?.agent_key || ROOT_AGENT_KEY;
  const agentName = firstAssistant?.agent_ref?.name || '精灵助手';
  const agentIcon = firstAssistant?.agent_ref?.icon || '精';

  // Calculate duration from message timestamps
  const startTs = userMessage?.created_at ? new Date(userMessage.created_at).getTime() : 0;
  const endTs = lastMsg?.created_at ? new Date(lastMsg.created_at).getTime() : 0;
  const durationMs = startTs && endTs ? Math.max(0, endTs - startTs) : null;

  // Extract reply text from the last assistant message (no inference)
  const replyContent = firstAssistant?.content_markdown || null;

  const agentWork: AgentWorkProcess = {
    agentKey,
    agentName,
    agentIcon,
    agentColor: agentColorFromKey(agentKey),
    status: 'completed',
    durationMs,
    activities: [],
    task: userMessage?.content_markdown || null,
    result: replyContent,
    hasPartialFailure: false,
    plan: null,
    teamStatus: null,
    progressSections: [],
    startedAt: userMessage?.created_at || '',
    finishedAt: lastMsg?.created_at || '',
    isLegacy: true,  // Signal to UI: this turn has no Activity data
  };

  return {
    id: `turn-legacy-${userMessage?.id || 'no-user'}`,
    userMessage: userMessage ?? null,
    agentWork,
  };
}

/**
 * Build a single ConversationTurn from raw Activity records for one turn.
 * Maps raw Activity records to TimelineActivity[] and constructs the
 * AgentWorkProcess without any message inference.
 */
function buildSingleTurnFromActivities(
  turn: UserTurn,
  rawRecords: readonly import('../activityTypes').Activity[],
  opts: { agentKey: string; taskContent: string | null; activityTree: ActivityTreeNode[]; progressByStep: Map<string, ProgressSection> },
): ConversationTurn {
  const userMessage = turn.userMessage || turn.messages.find((m) => m.role === 'user');
  const firstAssistant = turn.messages.find((m) => m.role === 'assistant' && !isTeamMemberOrigin(m.origin));

  // Build Activity tree from raw records for this turn
  const treeNodes = buildTreeFromRecords(rawRecords);
  let turnTimelineActivities = treeNodes
    .filter((node) => node.kind !== 'task' && node.kind !== 'sub_task_board' && node.kind !== 'delegate')
    .map(activityToTimelineActivity);

  // D3: Merge adjacent ThinkActivities into a single grouped ThinkActivity.
  // Complex tasks can produce 5-10 separate thinking steps (planning, reasoning,
  // replanning), which creates visual clutter. Merging them into one collapsible
  // group with subSteps reduces card count while preserving all content.
  turnTimelineActivities = mergeAdjacentThinkActivities(turnTimelineActivities);

  // Agent info from Activity data or fallback to message data
  const rootTask = rawRecords.find((r) => r.kind === 'task');
  const agentKey = rootTask?.agentKey || opts.agentKey || firstAssistant?.agent_ref?.agent_key || ROOT_AGENT_KEY;
  const agentName = rootTask?.agentName || firstAssistant?.agent_ref?.name || '精灵助手';
  const agentIcon = firstAssistant?.agent_ref?.icon || '精';

  // Determine status from activities
  const hasRunning = rawRecords.some((a) => a.status === 'running' || a.status === 'tool_running' || a.status === 'tool_blocked');
  const hasFailed = rawRecords.some((a) => a.kind === 'action' && a.status === 'failed');
  const status: AgentWorkProcess['status'] = hasRunning ? 'running' : (hasFailed ? 'failed' : 'completed');

  // Calculate duration
  const startTs = userMessage?.created_at ? new Date(userMessage.created_at).getTime() : 0;
  const lastMsg = turn.messages[turn.messages.length - 1];
  const endTs = lastMsg?.created_at ? new Date(lastMsg.created_at).getTime() : 0;
  const durationMs = startTs && endTs ? Math.max(0, endTs - startTs) : null;

  // Find last SayActivity for result
  const lastSay = [...turnTimelineActivities].reverse().find((a) => a.kind === 'say') as SayActivity | undefined;

  // Build progressSections
  const progressSections = buildProgressSections(opts.progressByStep, userMessage);

  const agentWork: AgentWorkProcess = {
    agentKey,
    agentName,
    agentIcon,
    agentColor: agentColorFromKey(agentKey),
    status,
    durationMs,
    activities: turnTimelineActivities,
    task: opts.taskContent || rootTask?.content || userMessage?.content_markdown || null,
    result: lastSay?.content || null,
    hasPartialFailure: false,
    plan: null,
    teamStatus: null,
    progressSections,
    startedAt: userMessage?.created_at || firstAssistant?.created_at || '',
    finishedAt: !hasRunning ? lastMsg?.created_at || '' : null,
  };

  return {
    id: `turn-${userMessage?.id || 'no-user'}`,
    userMessage: userMessage ?? null,
    agentWork,
  };
}

/**
 * D3: Merge adjacent ThinkActivities into a single grouped ThinkActivity.
 * Complex tasks can produce 5-10 separate thinking steps (planning, reasoning,
 * replanning), which creates visual clutter. Merging them into one collapsible
 * group with subSteps reduces card count while preserving all content.
 *
 * Rules:
 * - Only adjacent ThinkActivities are merged (interrupted by act/say → separate groups)
 * - A single ThinkActivity is never wrapped (no subSteps if only one)
 * - The merged content concatenates all sub-step content with newlines
 * - The merged durationMs is the sum of all sub-step durations
 * - The merged id uses the first sub-step's id with a "-merged" suffix
 */
function mergeAdjacentThinkActivities(activities: Activity[]): Activity[] {
  const result: Activity[] = [];
  let i = 0;
  while (i < activities.length) {
    const current = activities[i];
    if (current.kind !== 'think') {
      result.push(current);
      i++;
      continue;
    }
    // Collect adjacent ThinkActivities
    const group: ThinkActivity[] = [current as ThinkActivity];
    while (i + 1 < activities.length && activities[i + 1].kind === 'think') {
      i++;
      group.push(activities[i] as ThinkActivity);
    }
    if (group.length === 1) {
      // Single ThinkActivity — no merge needed
      result.push(group[0]);
    } else {
      // Merge: concatenate content, sum durations, collect subSteps
      const mergedContent = group.map((s) => s.content).filter(Boolean).join('\n\n');
      const totalDuration = group.reduce((sum, s) => sum + (s.durationMs ?? 0), 0) || null;
      const anyStreaming = group.some((s) => s.streaming);
      result.push({
        kind: 'think',
        id: group[0].id + '-merged',
        content: mergedContent,
        label: undefined,
        collapsed: !anyStreaming,
        streaming: anyStreaming,
        durationMs: totalDuration,
        subSteps: group,
      });
    }
    i++;
  }
  return result;
}

/**
 * Build a simple ActivityTreeNode[] from flat Activity records.
 * Uses parentActivityId to establish parent-child relationships.
 */
function buildTreeFromRecords(records: readonly import('../activityTypes').Activity[]): ActivityTreeNode[] {
  const nodeMap = new Map<string, ActivityTreeNode>();
  const roots: ActivityTreeNode[] = [];

  // First pass: create all nodes
  for (const record of records) {
    nodeMap.set(record.id, { ...record, children: [] });
  }

  // Second pass: establish parent-child relationships
  for (const record of records) {
    const node = nodeMap.get(record.id)!;
    if (record.parentActivityId && nodeMap.has(record.parentActivityId)) {
      nodeMap.get(record.parentActivityId)!.children.push(node);
    } else {
      roots.push(node);
    }
  }

  return roots;
}

// ── AF-FE-06: Build ConversationTurns from Activity-First data ──

/**
 * Build TeamPanel from Activity tree.
 * When the tree contains delegate/sub_task_board nodes, construct the
 * TaskBoard + DAG + TeamProgress sections for the unified panel.
 */
function buildTeamPanelFromActivityTree(tree: ActivityTreeNode[]): TeamPanel | undefined {
  if (!tree || tree.length === 0) return undefined;

  // Find delegate or sub_task_board nodes in the tree
  const delegateNodes = findNodesByKind(tree, 'delegate');
  const subTaskBoardNodes = findNodesByKind(tree, 'sub_task_board');
  const hasTeamStructure = delegateNodes.length > 0 || subTaskBoardNodes.length > 0;
  if (!hasTeamStructure) return undefined;

  // Build TaskBoard from root task and its children
  const rootTask = tree[0]; // root is always the task node
  const taskBoardEntries: TaskBoardSection['entries'] = [];
  let num = 1;

  // Add the root task itself
  taskBoardEntries.push({
    id: rootTask.id,
    task: rootTask.content || rootTask.label || '',
    status: mapActivityStatusToPlanStatus(rootTask.status),
    num: num++,
    agentKey: rootTask.agentKey || null,
    agentName: rootTask.agentName || null,
    agentIcon: rootTask.agentName?.charAt(0) || null,
    agentColor: agentColorFromKey(rootTask.agentKey || ''),
  });

  // Add delegate/sub_task_board children as task board entries
  for (const child of rootTask.children) {
    if (child.kind === 'delegate' || child.kind === 'sub_task_board') {
      taskBoardEntries.push({
        id: child.id,
        task: child.content || child.label || '',
        status: mapActivityStatusToPlanStatus(child.status),
        num: num++,
        agentKey: child.agentKey || null,
        agentName: child.agentName || null,
        agentIcon: child.agentName?.charAt(0) || null,
        agentColor: agentColorFromKey(child.agentKey || ''),
      });
    }
  }

  // Build DAG from dependsOn relationships
  const dagNodes: DagSection['nodes'] = [];
  const dagEdges: DagSection['edges'] = [];
  const allNodes = flattenTree(tree);
  for (const node of allNodes) {
    if (node.dagNodeId) {
      dagNodes.push({
        id: node.dagNodeId,
        label: node.label || node.content || '',
        status: mapActivityStatusToDagStatus(node.status),
      });
      if (node.dependsOn) {
        for (const depId of node.dependsOn) {
          dagEdges.push({ from: depId, to: node.dagNodeId });
        }
      }
    }
  }

  // Build TeamProgress from delegate/sub_task_board nodes
  const teamProgress: TeamProgressSection[] = [];
  for (const delegateNode of delegateNodes) {
    const agents = buildAgentProgress(delegateNode);
    const totalAgents = agents.length;
    const completedAgents = agents.filter((a) => a.status === 'completed').length;
    const progressPercent = totalAgents > 0 ? Math.round((completedAgents / totalAgents) * 100) : 0;

    teamProgress.push({
      teamId: delegateNode.teamId || delegateNode.id,
      teamName: delegateNode.label || delegateNode.content || '团队',
      teamIcon: delegateNode.agentName?.charAt(0) || 'T',
      status: mapActivityStatusToTeamStatus(delegateNode.status),
      progressPercent,
      durationMs: delegateNode.durationMs,
      agents,
      actions: delegateNode.status === 'interrupted' ? ['resume', 'cancel'] : undefined,
    });
  }
  for (const stbNode of subTaskBoardNodes) {
    const agents = buildAgentProgress(stbNode);
    const totalAgents = agents.length;
    const completedAgents = agents.filter((a) => a.status === 'completed').length;
    const progressPercent = totalAgents > 0 ? Math.round((completedAgents / totalAgents) * 100) : 0;

    teamProgress.push({
      teamId: stbNode.teamId || stbNode.id,
      teamName: stbNode.label || stbNode.content || '子任务',
      teamIcon: stbNode.agentName?.charAt(0) || 'S',
      status: mapActivityStatusToTeamStatus(stbNode.status),
      progressPercent,
      durationMs: stbNode.durationMs,
      agents,
    });
  }

  return {
    taskBoard: { entries: taskBoardEntries },
    dag: dagNodes.length > 0 ? { nodes: dagNodes, edges: dagEdges } : undefined,
    teamProgress,
  };
}

/** Find all nodes of a given kind in the tree (recursive). */
function findNodesByKind(tree: ActivityTreeNode[], kind: string): ActivityTreeNode[] {
  const result: ActivityTreeNode[] = [];
  for (const node of tree) {
    if (node.kind === kind) result.push(node);
    result.push(...findNodesByKind(node.children, kind));
  }
  return result;
}

/** Flatten tree into a flat list. */
function flattenTree(tree: ActivityTreeNode[]): ActivityTreeNode[] {
  const result: ActivityTreeNode[] = [];
  for (const node of tree) {
    result.push(node);
    result.push(...flattenTree(node.children));
  }
  return result;
}

/** Build AgentProgress from a delegate/sub_task_board node's children. */
function buildAgentProgress(parentNode: ActivityTreeNode): AgentProgress[] {
  const agents: AgentProgress[] = [];
  // Group children by agentKey
  const agentMap = new Map<string, ActivityTreeNode[]>();
  for (const child of parentNode.children) {
    const key = child.agentKey || child.id;
    if (!agentMap.has(key)) agentMap.set(key, []);
    agentMap.get(key)!.push(child);
  }

  for (const [key, children] of agentMap) {
    const firstChild = children[0];
    const agentActivities: Activity[] = children.map((child) => activityToTimelineActivity(child));
    const hasRunning = children.some((c) => c.status === 'running' || c.status === 'tool_running');
    const hasFailed = children.some((c) => c.status === 'failed');
    const allCompleted = children.every((c) => c.status === 'completed');

    agents.push({
      agentKey: key,
      agentName: firstChild.agentName || key,
      agentIcon: firstChild.agentName?.charAt(0) || key.charAt(0),
      status: hasRunning ? 'running' : (hasFailed ? 'failed' : (allCompleted ? 'completed' : 'waiting')),
      activities: agentActivities,
    });
  }

  return agents;
}

/** Map ActivityStatus to PlanEntry status for TaskBoard. */
function mapActivityStatusToPlanStatus(status: string): 'pending' | 'running' | 'completed' | 'failed' {
  switch (status) {
    case 'completed': return 'completed';
    case 'failed':
    case 'partial_failure': return 'failed';
    case 'running':
    case 'tool_running':
    case 'tool_blocked': return 'running';
    default: return 'pending';
  }
}

/** Map ActivityStatus to DAG node status. */
function mapActivityStatusToDagStatus(status: string): 'done' | 'running' | 'pending' | 'failed' {
  switch (status) {
    case 'completed': return 'done';
    case 'failed':
    case 'partial_failure': return 'failed';
    case 'running':
    case 'tool_running':
    case 'tool_blocked': return 'running';
    default: return 'pending';
  }
}

/** Map ActivityStatus to TeamProgressSection status. */
function mapActivityStatusToTeamStatus(status: string): 'running' | 'completed' | 'failed' | 'interrupted' {
  switch (status) {
    case 'completed': return 'completed';
    case 'failed':
    case 'partial_failure': return 'failed';
    case 'interrupted':
    case 'cancelled': return 'interrupted';
    default: return 'running';
  }
}

/**
 * Build TaskBoardNodeData[] from Activity tree for tree-nested rendering.
 * Reuses the activityToTaskBoardNode mapping from useActivityTimeline.
 */
function buildTaskBoardNodesFromActivityTree(tree: ActivityTreeNode[]): TaskBoardNodeData[] | undefined {
  if (!tree || tree.length === 0) return undefined;
  const nodes = tree
    .filter((node) => node.kind !== 'delegate' && node.kind !== 'notice')
    .map(activityToTaskBoardNode);
  return nodes.length > 0 ? nodes : undefined;
}
