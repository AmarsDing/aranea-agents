/**
 * useConversationTimeline — 从 Activity 事件构建 ConversationTurn[]
 *
 * 单路径架构（Activity-First）：
 * - 当 activityRawRecords 非空时，从后端 Activity 事件构建 Turn，零推理。
 * - 无 AF 数据时，从原始消息构建最小化 Turn（isLegacy=true），不做推理。
 *
 * 设计原则：
 * 1. AF 路径：按 turnId 分组 Activity 记录，构建完整 Think/Act/Say 时间线
 * 2. Legacy 路径：按 role=user 边界划分 Turn，仅提取基本信息，标记 isLegacy
 * 3. isFinal：Turn 内最后一个 SayActivity 的 isFinal=true
 */

import { computed, type ComputedRef } from 'vue';
import type { Message } from '../types';
import type { Envelope } from '../../../realtime/envelope';
import type {
  ConversationTurn,
  AgentWorkProcess,
  Activity,
} from '../activityTimelineTypes';
import type { ReplyEvent, ThinkingEvent } from '../streamEventTypes';
import type { ActivityTreeNode } from '../activityTypes';
import type { AgentBlock, ProgressSection } from '../agentTreeTypes';
import { agentColorFromKey, ROOT_AGENT_KEY } from '../agentTreeTypes';
import { activityToStreamEvent } from './useActivityTimeline';
import { isTeamMemberOrigin, ensureOrigin } from '../messageOrigin';
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

/** 按 role=user 边界划分消息为 UserTurn[] */
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

// ── Main composable ──

export function useConversationTimeline(deps: {
  messages: ComputedRef<Message[]>;
  isTeamSession?: boolean;
  plannerKind?: ComputedRef<string>;
  progressEnvelopes?: ComputedRef<readonly Envelope[]>;
  activityTimelineActivities?: ComputedRef<readonly Activity[]>;
  activityAgentKey?: ComputedRef<string>;
  activityTaskContent?: ComputedRef<string | null>;
  activityTree?: ComputedRef<readonly ActivityTreeNode[]>;
  activityRawRecords?: ComputedRef<readonly import('../activityTypes').Activity[]>;
}) {
  const conversationTurns = computed((): ConversationTurn[] => {
    const allMessages = deps.messages.value;
    if (allMessages.length === 0) return [];

    // Merge execution_progress envelopes into ProgressSection map
    const progressByStep = deps.progressEnvelopes?.value
      ? mergeProgressEvents(deps.progressEnvelopes.value)
      : new Map<string, ProgressSection>();

    const plannerKind = deps.plannerKind?.value ?? '';

    // AF-FE-06 + AF-FE-14: When Activity-First data is available, build turns
    // from Activity events. Uses buildAllConversationTurnsFromActivities which
    // groups raw Activity records by turnId to build ALL turns from AF data.
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
        plannerKind,
      });
    }

    // No AF data — build minimal legacy turns (no inference)
    const ensured = allMessages.map((m) => ensureOrigin(m));
    const turns = findUserTurns(ensured);
    return turns.map((turn) => buildLegacyConversationTurn(turn, {
      agentKey: deps.activityAgentKey?.value || '',
      taskContent: deps.activityTaskContent?.value || null,
      activityTree: [],
      progressByStep,
    }));
  });

  return {
    conversationTurns,
    /** @deprecated AgentBlocks removed in AF refactoring — always empty */
    agentBlocks: computed((): AgentBlock[] => []),
  };
}

/**
 * Build a minimal ConversationTurn for turns without Activity data.
 * Shows the user message and basic agent info. No message inference.
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
    isLegacy: true,
  };

  return {
    id: `turn-legacy-${userMessage?.id || 'no-user'}`,
    userMessage: userMessage ?? null,
    agentWork,
  };
}

// ── AF-FE-14: Build ALL ConversationTurns from raw Activity records ──

function buildAllConversationTurnsFromActivities(
  messages: Message[],
  rawRecords: readonly import('../activityTypes').Activity[],
  timelineActivities: readonly Activity[],
  opts: { agentKey: string; taskContent: string | null; activityTree: ActivityTreeNode[]; progressByStep: Map<string, ProgressSection>; plannerKind: string },
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
    const turnId = userMessage?.turn_id
      || turn.messages.find((m) => m.role === 'assistant')?.turn_id
      || '';

    const turnActivities = turnId ? activitiesByTurn.get(turnId) : undefined;

    if (turnActivities && turnActivities.length > 0) {
      // Build from Activity data — zero inference
      result.push(buildSingleTurnFromActivities(turn, turnActivities, opts));
    } else {
      // No Activity data for this turn — legacy fallback
      result.push(buildLegacyConversationTurn(turn, opts));
    }
  }

  return result;
}

function buildSingleTurnFromActivities(
  turn: UserTurn,
  rawRecords: readonly import('../activityTypes').Activity[],
  opts: { agentKey: string; taskContent: string | null; activityTree: ActivityTreeNode[]; progressByStep: Map<string, ProgressSection> },
): ConversationTurn {
  const userMessage = turn.userMessage || turn.messages.find((m) => m.role === 'user');
  const firstAssistant = turn.messages.find((m) => m.role === 'assistant' && !isTeamMemberOrigin(m.origin));

  // Build Activity tree from raw records for this turn
  const treeNodes = buildTreeFromRecords(rawRecords);
  const allNodes = flattenTree(treeNodes);
  let turnTimelineActivities: Activity[] = allNodes
    .filter((node) => node.kind !== 'task' && node.kind !== 'sub_task_board' && node.kind !== 'delegate')
    .map(activityToStreamEvent);

  // D3: Merge adjacent ThinkActivities
  turnTimelineActivities = mergeAdjacentThinkActivities(turnTimelineActivities);

  const rootTask = rawRecords.find((r) => r.kind === 'task');
  const agentKey = rootTask?.agentKey || opts.agentKey || firstAssistant?.agent_ref?.agent_key || ROOT_AGENT_KEY;
  const agentName = rootTask?.agentName || firstAssistant?.agent_ref?.name || '精灵助手';
  const agentIcon = rootTask?.agentName?.charAt(0) || firstAssistant?.agent_ref?.icon || '精';

  // Determine status from activities
  const hasRunning = rawRecords.some((a) => a.status === 'running' || a.status === 'tool_running' || a.status === 'tool_blocked');
  const hasFailedAction = rawRecords.some((a) => a.kind === 'action' && a.status === 'failed');
  const hasError = rawRecords.some((a) => a.kind === 'error');
  const rootTaskCompleted = rootTask?.status === 'completed';
  const lastSay = [...turnTimelineActivities].reverse().find((a) => a.kind === 'reply') as ReplyEvent | undefined;
  const hasResult = lastSay != null && !!lastSay.content;
  const isPartialFailure = hasFailedAction && rootTaskCompleted && hasResult;
  let status: AgentWorkProcess['status'];
  if (hasRunning) {
    status = 'running';
  } else if (hasError || (hasFailedAction && !hasResult)) {
    status = 'failed';
  } else {
    status = 'completed';
  }

  // Calculate duration
  const startTs = userMessage?.created_at ? new Date(userMessage.created_at).getTime() : 0;
  const lastMsg = turn.messages[turn.messages.length - 1];
  const endTs = lastMsg?.created_at ? new Date(lastMsg.created_at).getTime() : 0;
  const durationMs = startTs && endTs ? Math.max(0, endTs - startTs) : null;

  const progressSections = buildProgressSections(opts.progressByStep, userMessage);

  const agentWork: AgentWorkProcess = {
    agentKey,
    agentName,
    agentIcon,
    agentColor: agentColorFromKey(agentKey),
    status,
    durationMs,
    activities: turnTimelineActivities,
    activityTree: opts.activityTree,
    task: opts.taskContent || rootTask?.content || userMessage?.content_markdown || null,
    result: lastSay?.content || null,
    hasPartialFailure: isPartialFailure,
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

function mergeAdjacentThinkActivities(activities: Activity[]): Activity[] {
  const result: Activity[] = [];
  let i = 0;
  while (i < activities.length) {
    const current = activities[i];
    if (current.kind !== 'thinking') {
      result.push(current);
      i++;
      continue;
    }
    const group: ThinkingEvent[] = [current as ThinkingEvent];
    while (i + 1 < activities.length && activities[i + 1].kind === 'thinking') {
      i++;
      group.push(activities[i] as ThinkingEvent);
    }
    if (group.length === 1) {
      result.push(group[0]);
    } else {
      const mergedContent = group.map((s) => s.content).filter(Boolean).join('\n\n');
      const totalDuration = group.reduce((sum, s) => sum + (s.durationMs ?? 0), 0) || null;
      const anyStreaming = group.some((s) => s.streaming);
      result.push({
        kind: 'thinking',
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

function buildTreeFromRecords(records: readonly import('../activityTypes').Activity[]): ActivityTreeNode[] {
  const nodeMap = new Map<string, ActivityTreeNode>();
  const roots: ActivityTreeNode[] = [];

  for (const record of records) {
    nodeMap.set(record.id, { ...record, children: [] });
  }

  for (const record of records) {
    const node = nodeMap.get(record.id)!;
    if (record.parentActivityId && nodeMap.has(record.parentActivityId)) {
      nodeMap.get(record.parentActivityId)!.children.push(node);
    } else {
      roots.push(node);
    }
  }

  const sortTree = (nodes: ActivityTreeNode[]) => {
    nodes.sort((a, b) => a.timestamp.localeCompare(b.timestamp));
    for (const node of nodes) sortTree(node.children);
  };
  sortTree(roots);

  return roots;
}

function flattenTree(tree: ActivityTreeNode[]): ActivityTreeNode[] {
  const result: ActivityTreeNode[] = [];
  for (const node of tree) {
    result.push(node);
    result.push(...flattenTree(node.children));
  }
  return result;
}


