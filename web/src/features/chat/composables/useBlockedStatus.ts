/**
 * 阻塞状态检测 composable
 *
 * 基于后端状态机精确定义阻塞（非纯时间推断）。
 * 4 种阻塞类型：
 * - tool: Activity(kind=action, status=tool_running) + 无 ToolResult
 * - llm: Activity(kind=thinking/reply, status=running) + 无 streaming
 * - confirm: Activity(kind=confirm, status=tool_blocked)
 * - hitl: TeamRun(status=waiting_human) + Activity(kind=confirm, status=tool_blocked)
 *
 * 详见设计文档 §B.8.3 阻塞状态精确定义。
 */
import { computed, type Ref } from 'vue';
import type { ActivityTreeNode, ActivityStatus } from '../activityTypes';
import type { BlockedType } from '../streamEventTypes';

export interface BlockedResult {
  blocked: boolean;
  type: BlockedType | null;
  message: string;
  activityId: string | null;
  /** 阻塞活动所属的 agentKey，用于左侧 Agent 卡片精确高亮 */
  agentKey: string | null;
}

export const EMPTY_BLOCKED: BlockedResult = {
  blocked: false,
  type: null,
  message: '',
  activityId: null,
  agentKey: null,
};

export interface DetectOptions {
  /** 父级 agentKey，用于子活动未携带 agentKey 时继承 */
  parentAgentKey?: string | null;
}

/**
 * 判断单个活动节点是否处于阻塞状态。
 *
 * 判定规则（基于后端状态机，不依赖时间）：
 * 1. tool_running: 工具调用中但无 ToolResult → 工具阻塞
 * 2. tool_blocked: 等待用户确认 → 确认阻塞
 * 3. LLM 阻塞：仅当后端显式标记 meta.streaming === false 且状态仍为 running 时判定
 *    （避免正常流式生成被误判为阻塞）。
 *
 * 注意：此函数仅基于 Activity 状态判定，不依赖时间。
 */
export function detectBlockedActivity(node: ActivityTreeNode, options?: DetectOptions): BlockedResult {
  const status: ActivityStatus = node.status;
  const inheritedAgentKey = options?.parentAgentKey ?? null;
  const effectiveAgentKey = node.agentKey ?? inheritedAgentKey;

  // 工具阻塞：action + tool_running
  if (node.kind === 'action' && status === 'tool_running') {
    return {
      blocked: true,
      type: 'tool',
      message: `工具阻塞 · 等待 ${node.toolName || '工具'} 返回`,
      activityId: node.id,
      agentKey: effectiveAgentKey,
    };
  }

  // 确认阻塞：confirm + tool_blocked
  if (node.kind === 'confirm' && status === 'tool_blocked') {
    return {
      blocked: true,
      type: 'confirm',
      message: '等待用户确认',
      activityId: node.id,
      agentKey: effectiveAgentKey,
    };
  }

  // LLM 阻塞：需要后端显式标记当前无 streaming 且心跳仍存活。
  // 前端不自行根据时间推断；只有 meta.streaming === false 时才视为 LLM 阻塞。
  if ((node.kind === 'thinking' || node.kind === 'reply') && status === 'running' && node.meta?.streaming === false) {
    return {
      blocked: true,
      type: 'llm',
      message: 'LLM 阻塞 · 等待模型响应',
      activityId: node.id,
      agentKey: effectiveAgentKey,
    };
  }

  return EMPTY_BLOCKED;
}

/**
 * 遍历活动树，查找第一个阻塞节点。
 * 优先级：tool > confirm > llm（工具阻塞最常见，优先返回）。
 * 子活动未携带 agentKey 时继承父节点 agentKey，使左侧 Agent 卡片能正确高亮。
 */
export function findBlockedInTree(nodes: ActivityTreeNode[], parentAgentKey?: string | null): BlockedResult {
  if (!nodes?.length) return EMPTY_BLOCKED;

  // 先查工具阻塞和确认阻塞（确定性更高）
  for (const node of nodes) {
    const effectiveAgentKey = node.agentKey ?? parentAgentKey ?? null;
    if (node.children?.length) {
      const childResult = findBlockedInTree(node.children, effectiveAgentKey);
      if (childResult.blocked) return childResult;
    }
    const result = detectBlockedActivity(node, { parentAgentKey: effectiveAgentKey });
    if (result.blocked && (result.type === 'tool' || result.type === 'confirm')) {
      return result;
    }
  }

  // 再查 LLM 阻塞（需后端显式标记无 streaming）
  for (const node of nodes) {
    const effectiveAgentKey = node.agentKey ?? parentAgentKey ?? null;
    if (node.children?.length) {
      const childResult = findBlockedInTree(node.children, effectiveAgentKey);
      if (childResult.blocked) return childResult;
    }
    const result = detectBlockedActivity(node, { parentAgentKey: effectiveAgentKey });
    if (result.blocked) return result;
  }

  return EMPTY_BLOCKED;
}

/**
 * 阻塞状态检测 composable。
 *
 * @param activityTree 当前 session 的活动树
 * @returns 阻塞检测结果
 */
export function useBlockedStatus(activityTree: Ref<ActivityTreeNode[]>) {
  const blocked = computed(() => findBlockedInTree(activityTree.value));
  return blocked;
}
