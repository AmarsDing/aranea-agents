/**
 * useConversationTimeline — 从 Activity 事件构建 ConversationTurn[]
 *
 * 单路径架构（Activity-First，T7.3a 已移除 Legacy 路径）：
 * - 当 activityRawRecords 非空时，从后端 Activity 事件构建 Turn，零推理。
 * - 无 AF 数据时，返回空数组（pre-AF 会话由 T6.3 回填保证有 AF 数据；
 *   AF API 失败时由 AF-GAP-05 显示错误提示）。
 *
 * 设计原则：
 * 1. AF 路径：按 turnId 分组 Activity 记录，构建完整 Think/Act/Say 时间线
 * 2. isFinal：Turn 内最后一个 SayActivity 的 isFinal=true
 */

import { computed, type ComputedRef } from 'vue';
import type { Message } from '../types';
import type { Envelope } from '../../../realtime/envelope';
import type { ConversationTurn, AgentWorkProcess, Activity } from '../activityTimelineTypes';
import type { ReplyEvent } from '../streamEventTypes';
import type { ActivityTreeNode } from '../activityTypes';
import type { AgentBlock, ProgressSection } from '../agentTreeTypes';
import { agentColorFromKey, ROOT_AGENT_KEY } from '../agentTreeTypes';
import { activityToStreamEvent } from './useActivityTimeline';
import { isTeamMemberOrigin, ensureOrigin } from '../messageOrigin';
import { mergeProgressEvents } from '../executionProgress';
import { useAppStore } from '../../../stores/app';

// ── UserTurn (internal) ──

interface UserTurn {
  userMessage: Message | null;
  messages: Message[];
}

// ── Helpers ──

/** 从 appStore.agents 按 agent_key 查询 agent.icon，用于补全 Activity 重建消息缺失的 icon。 */
function resolveAgentIconFromStore(agentKey: string): string {
  const key = agentKey?.trim();
  if (!key || key === ROOT_AGENT_KEY) return '';
  try {
    return useAppStore().agents.find((a) => a.agent_key === key)?.icon ?? '';
  } catch {
    return '';
  }
}

/** Build sorted ProgressSection[] from execution_progress envelopes, clamping startedAt to turn start. */
function buildProgressSections(
  progressByStep: Map<string, ProgressSection>,
  userMessage: Message | undefined,
): ProgressSection[] {
  if (progressByStep.size === 0) return [];
  const turnStartTs = userMessage?.created_at ? new Date(userMessage.created_at).getTime() : Date.now();
  const sections: ProgressSection[] = [];
  for (const section of progressByStep.values()) {
    // Clamp startedAt to turn start to avoid progress events appearing before the turn
    sections.push({ ...section, startedAt: Math.max(turnStartTs, section.startedAt) });
  }
  sections.sort((a, b) => a.startedAt - b.startedAt);
  return sections;
}

/** 按 role=user 边界划分消息为 UserTurn[]（turn 边界辅助函数，AF 唯一路径下使用） */
function splitByUserMessages(messages: Message[]): UserTurn[] {
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
  // Per-turn memoization: historical turns keep the same object reference as
  // long as their inputs haven't changed. This avoids DynamicScroller / Vue
  // diffing the entire list on every streaming delta.
  const turnCache = new Map<string, { signature: string; turn: ConversationTurn }>();

  const conversationTurns = computed((): ConversationTurn[] => {
    const allMessages = deps.messages.value;
    if (allMessages.length === 0) {
      turnCache.clear();
      return [];
    }

    // Merge execution_progress envelopes into ProgressSection map
    const progressByStep = deps.progressEnvelopes?.value
      ? mergeProgressEvents(deps.progressEnvelopes.value)
      : new Map<string, ProgressSection>();

    const plannerKind = deps.plannerKind?.value ?? '';

    const afTree = deps.activityTree?.value;
    const afRawRecords = deps.activityRawRecords?.value;

    const ensured = allMessages.map(ensureOrigin);
    const userTurns = splitByUserMessages(ensured);
    if (userTurns.length === 0) return [];

    const activitiesByTurn = groupRawRecordsByTurn(afRawRecords ?? []);

    const agentKey = deps.activityAgentKey?.value || '';
    const taskContent = deps.activityTaskContent?.value || null;
    const activityTree = afTree ? [...afTree] : [];
    const activityTreeSig = computeActivityTreeSignature(activityTree);

    const result: ConversationTurn[] = [];
    const seenTurnKeys = new Set<string>();

    for (const turn of userTurns) {
      const userMessage = turn.userMessage || turn.messages.find((m) => m.role === 'user');
      const turnId = userMessage?.turn_id || turn.messages.find((m) => m.role === 'assistant')?.turn_id || '';
      const turnActivities = turnId ? activitiesByTurn.get(turnId) : undefined;

      const cacheKey = turnId || `turn-${userMessage?.id || 'no-user'}`;
      const signature = buildTurnSignature(turn, turnActivities ?? [], {
        agentKey,
        taskContent,
        activityTreeSig,
        progressByStep,
        plannerKind,
      });

      const cached = turnCache.get(cacheKey);
      if (cached && cached.signature === signature) {
        result.push(cached.turn);
        seenTurnKeys.add(cacheKey);
        continue;
      }

      const built = buildSingleTurnFromActivities(turn, turnActivities ?? [], {
        agentKey,
        taskContent,
        activityTree,
        progressByStep,
      });
      turnCache.set(cacheKey, { signature, turn: built });
      result.push(built);
      seenTurnKeys.add(cacheKey);
    }

    // Remove cache entries for turns that no longer exist (e.g. session switch).
    for (const key of turnCache.keys()) {
      if (!seenTurnKeys.has(key)) {
        turnCache.delete(key);
      }
    }

    return result;
  });

  return {
    conversationTurns,
    /** @deprecated AgentBlocks removed in AF refactoring — always empty */
    agentBlocks: computed((): AgentBlock[] => []),
  };
}

function groupRawRecordsByTurn(
  rawRecords: readonly import('../activityTypes').Activity[],
): Map<string, import('../activityTypes').Activity[]> {
  const map = new Map<string, import('../activityTypes').Activity[]>();
  for (const record of rawRecords) {
    const tid = record.turnId;
    if (!tid) continue;
    const group = map.get(tid);
    if (group) group.push(record);
    else map.set(tid, [record]);
  }
  return map;
}

function buildTurnSignature(
  turn: UserTurn,
  rawRecords: readonly import('../activityTypes').Activity[],
  opts: {
    agentKey: string;
    taskContent: string | null;
    activityTreeSig: string;
    progressByStep: Map<string, ProgressSection>;
    plannerKind: string;
  },
): string {
  const parts: string[] = [];
  parts.push(`u=${turn.userMessage?.id ?? 'n'}`);
  for (const m of turn.messages) {
    // Include content length and status; full content comparison is avoided
    // to keep signatures compact, while still catching meaningful changes.
    parts.push(`m=${m.id}:${m.status}:${m.turn_id}:${m.content_markdown.length}`);
  }
  parts.push(
    `r=${rawRecords
      .map((r) => {
        // Include kind-specific payload lengths so streaming deltas for
        // thinking/action/reply all invalidate the turn cache correctly.
        let payloadLen = (r.content ?? '').length;
        if (r.kind === 'thinking') {
          payloadLen = (r.reasoning ?? '').length;
        } else if (r.kind === 'action') {
          payloadLen = (r.toolArguments ?? '').length + (r.toolResult ?? '').length;
        }
        return `${r.id}:${r.status}:${r.kind}:${payloadLen}`;
      })
      .join(',')}`,
  );
  parts.push(`ak=${opts.agentKey}`);
  parts.push(`tc=${opts.taskContent ?? ''}`);
  parts.push(`pl=${opts.plannerKind}`);
  parts.push(`ps=${opts.progressByStep.size}`);
  parts.push(`at=${opts.activityTreeSig}`);
  return parts.join('|');
}

function computeActivityTreeSignature(tree: ActivityTreeNode[]): string {
  const parts: string[] = [];
  function walk(nodes: ActivityTreeNode[]) {
    for (const node of nodes) {
      parts.push(`${node.id}:${node.status}:${node.kind}`);
      walk(node.children);
    }
  }
  walk(tree);
  return parts.join(',');
}

// ── AF-FE-14: Build ALL ConversationTurns from raw Activity records ──

function buildSingleTurnFromActivities(
  turn: UserTurn,
  rawRecords: readonly import('../activityTypes').Activity[],
  opts: {
    agentKey: string;
    taskContent: string | null;
    activityTree: ActivityTreeNode[];
    progressByStep: Map<string, ProgressSection>;
  },
): ConversationTurn {
  const userMessage = turn.userMessage || turn.messages.find((m) => m.role === 'user');
  const firstAssistant = turn.messages.find((m) => m.role === 'assistant' && !isTeamMemberOrigin(m.origin));

  // Build Activity tree from raw records for this turn
  const treeNodes = buildTreeFromRecords(rawRecords);
  const allNodes = flattenTree(treeNodes);
  const turnTimelineActivities: Activity[] = allNodes
    .filter((node) => node.kind !== 'task' && node.kind !== 'sub_task_board' && node.kind !== 'delegate')
    .map(activityToStreamEvent);

  const rootTask = rawRecords.find((r) => r.kind === 'task');
  const agentKey = rootTask?.agentKey || opts.agentKey || firstAssistant?.agent_ref?.agent_key || ROOT_AGENT_KEY;
  const agentName = rootTask?.agentName || firstAssistant?.agent_ref?.name || '精灵助手';
  // 优先使用 message options_json 中的 agent.icon（后端 AssistantOptionsJSON 写入）；
  // 为空时回退到 appStore.agents 按 agent_key 查询（覆盖 Activity 重建消息 icon 缺失的场景）；
  // 仍为空时由 AgentAvatarQ 回退到 smart_toy 图标，与 AgentCard 保持一致。
  const refIcon = firstAssistant?.agent_ref?.icon || '';
  const agentIcon = refIcon || resolveAgentIconFromStore(agentKey);

  // Determine status from activities
  const hasRunning = rawRecords.some(
    (a) => a.status === 'running' || a.status === 'tool_running' || a.status === 'tool_blocked',
  );
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
  } else if (rawRecords.length === 0) {
    // 修复：无 Activity 数据时，根据用户消息状态决定 agentWork.status
    // - pending-user 占位消息（status='ok'）→ 'running'（显示"正在思考…"等待指示器）
    // - 失败消息（status='failed'）→ 'failed'（显示失败状态）
    // 这确保用户发送消息后能立即看到 UI 反馈，而不是空白
    status = userMessage?.status === 'failed' ? 'failed' : 'running';
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
    finishedAt: status !== 'running' ? lastMsg?.created_at || '' : null,
  };

  return {
    id: `turn-${userMessage?.id || 'no-user'}`,
    userMessage: userMessage ?? null,
    agentWork,
  };
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
    nodes.sort(compareActivities);
    for (const node of nodes) sortTree(node.children);
  };
  sortTree(roots);

  return roots;
}

/** Stable activity comparator: backend seq ASC, then timestamp ASC.
 *
 * `_seq` is the projector's global emission counter, so it is the most
 * reliable ordering signal. Timestamp strings are compared numerically as a
 * fallback because RFC3339Nano strings with variable fractional-digit lengths
 * do not sort lexicographically (e.g. `.100` vs `.99`). */
function compareActivities(a: ActivityTreeNode, b: ActivityTreeNode): number {
  const sa = a.seq ?? 0;
  const sb = b.seq ?? 0;
  if (sa !== sb) return sa - sb;

  const ta = new Date(a.timestamp).getTime();
  const tb = new Date(b.timestamp).getTime();
  if (!Number.isNaN(ta) && !Number.isNaN(tb) && ta !== tb) {
    return ta - tb;
  }

  return a.timestamp.localeCompare(b.timestamp);
}

function flattenTree(tree: ActivityTreeNode[]): ActivityTreeNode[] {
  const result: ActivityTreeNode[] = [];
  for (const node of tree) {
    result.push(node);
    result.push(...flattenTree(node.children));
  }
  return result;
}
