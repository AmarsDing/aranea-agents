// web/src/features/chat/composables/useObserveGraph.ts
import { computed, type Ref } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import { usePlanDAGLayout } from './usePlanDAGLayout';
import { useNodeOutputStore } from '../../../stores/chat/nodeOutputStore';
import type { GraphStage, GraphNode, TeamStage, MemberSession } from '../v2Types';

const DAG_OPTS = {
  width: 800,
  nodeWidth: 220,
  nodeHeight: 140,
  gapX: 60,
  gapY: 80,
  padX: 40,
};

/** 节点成员信息（从 TeamStage.Members / MemberSession 提取） */
export interface NodeMember {
  agentKey: string;
  agentName: string;
  avatarUrl?: string;
  status: string; // MemberSessionStatus
  currentAction?: string;
}

/**
 * useObserveGraph transforms activityV2Store data into a format suitable
 * for the ObservationCanvas (Vue Flow). It extracts the GraphStage and its
 * nodes, then computes layout positions using the existing DAG layout algorithm.
 *
 * Enhanced: extracts member info, duration, error, and current action from
 * TeamStage / MemberSession / Step data in activityV2Store.
 */
/**
 * useObserveNodeEnrichment exposes per-node enrichment helpers (members,
 * duration, error) shared by the canvas nodes and the node detail sidebar.
 * Must be called within a component setup scope (uses Pinia store).
 */
export function useObserveNodeEnrichment() {
  const activityStore = useChatActivityStore();

  /**
   * Find the TeamStage associated with a GraphNode.
   * Uses TeamStageID if set; falls back to DagNodeID matching.
   */
  function findTeamStage(node: GraphNode): TeamStage | undefined {
    if (node.TeamStageID) {
      return activityStore.teamStages.get(node.TeamStageID);
    }
    // Fallback: match by DagNodeID (= PlanStep.ID)
    for (const [, ts] of activityStore.teamStages) {
      if (ts.DagNodeID === node.DagNodeID) return ts;
    }
    return undefined;
  }

  /**
   * Extract node description from PlanStep or TeamStage.
   * Returns the sub-task description for display in the node body.
   */
  function extractDescription(node: GraphNode): string | undefined {
    // Try to get description from TeamStage first
    const ts = findTeamStage(node);
    if (ts?.Description) return ts.Description;
    // Fallback to node label if no description available
    return undefined;
  }

  /**
   * Extract text output summary for completed nodes.
   * Returns first 200 chars of the final reply content.
   */
  function extractTextOutput(node: GraphNode): string | undefined {
    if (node.Status !== 'completed') return undefined;
    const ts = findTeamStage(node);
    if (!ts) return undefined;

    // Get the final reply from TeamRuns
    const teamRuns = activityStore.getTeamStageTeamRuns(ts.ID);
    for (const tr of teamRuns) {
      if (tr.FinalReply) {
        return tr.FinalReply.slice(0, 200);
      }
    }

    // Fallback: check member sessions for final reply
    for (const tr of teamRuns) {
      const sessions = activityStore.getTeamRunMemberSessions(tr.ID);
      for (const ms of sessions) {
        if (ms.FinalReply) {
          return ms.FinalReply.slice(0, 200);
        }
      }
    }
    return undefined;
  }

  /**
   * Extract member info from TeamStage.Members and MemberSessions.
   * MemberInfo has agent key/name/avatar/status; MemberSessions add currentAction.
   */
  function extractMembers(node: GraphNode): NodeMember[] {
    const ts = findTeamStage(node);
    if (!ts || !ts.Members || ts.Members.length === 0) return [];

    // Build lookup for MemberSessions by agentKey
    const msByAgent = new Map<string, MemberSession>();
    const teamRuns = activityStore.getTeamStageTeamRuns(ts.ID);
    for (const tr of teamRuns) {
      const sessions = activityStore.getTeamRunMemberSessions(tr.ID);
      for (const ms of sessions) {
        msByAgent.set(ms.AgentKey, ms);
      }
    }

    return ts.Members.map((m) => {
      const ms = msByAgent.get(m.AgentKey);
      // Extract latest action from member's steps
      let currentAction: string | undefined;
      if (ms && ms.Status === 'running') {
        const memberSteps = activityStore.getMemberSessionSteps(ms);
        const latestAction = memberSteps
          .filter((s) => s.Kind === 'action' && s.Status === 'running')
          .sort((a, b) => b.Seq - a.Seq)[0];
        if (latestAction) {
          currentAction = latestAction.ToolName || latestAction.Content?.slice(0, 50);
        }
      }
      return {
        agentKey: m.AgentKey,
        agentName: m.AgentName,
        avatarUrl: m.AvatarURL || undefined,
        status: ms?.Status || m.Status || 'pending',
        currentAction,
      };
    });
  }

  /**
   * Calculate duration for a node.
   * running: elapsed from startedAt to now.
   * completed/failed: total duration from startedAt to completedAt.
   */
  function extractDuration(node: GraphNode): number | undefined {
    const ts = findTeamStage(node);
    if (!ts || !ts.StartedAt) return undefined;
    const start = new Date(ts.StartedAt).getTime();
    if (Number.isNaN(start)) return undefined;
    if (ts.CompletedAt) {
      const end = new Date(ts.CompletedAt).getTime();
      return Number.isNaN(end) ? undefined : end - start;
    }
    if (ts.Status === 'running') {
      return Date.now() - start;
    }
    return undefined;
  }

  /**
   * Extract error message for a failed node.
   * Checks TeamRun.Error first, then MemberSession.Error.
   */
  function extractError(node: GraphNode): string | undefined {
    if (node.Status !== 'failed') return undefined;
    const ts = findTeamStage(node);
    if (!ts) return undefined;
    // Check TeamRuns for error
    const teamRuns = activityStore.getTeamStageTeamRuns(ts.ID);
    for (const tr of teamRuns) {
      if (tr.Error) return tr.Error;
    }
    // Check MemberSessions for error
    for (const tr of teamRuns) {
      const sessions = activityStore.getTeamRunMemberSessions(tr.ID);
      for (const ms of sessions) {
        if (ms.Error) return ms.Error;
      }
    }
    return undefined;
  }

  return { findTeamStage, extractMembers, extractDuration, extractError, extractDescription, extractTextOutput };
}

export function useObserveGraph(spiritSessionId: Ref<string>) {
  const activityStore = useChatActivityStore();
  const nodeOutputStore = useNodeOutputStore();
  const { extractMembers, extractDuration, extractError, extractDescription, extractTextOutput } =
    useObserveNodeEnrichment();

  /**
   * Extract the latest media progress for a node from activity steps.
   * Steps carry no TeamStageID; matching follows the same convention as
   * mediaOutput sync in activityV2Store.upsertStep (AuthorAgentKey → node key).
   * Progress is read defensively from ToolResult.progress (backend progress
   * push is still a stub); returns undefined when nothing usable is found.
   */
  function extractProgress(nodeKey: string): { value: number; max: number; label?: string } | undefined {
    let latest: { value: number; max: number; label?: string } | undefined;
    let latestSeq = -1;
    for (const step of activityStore.steps.values()) {
      if (step.AuthorAgentKey !== nodeKey) continue;
      const p = (step.ToolResult as Record<string, unknown> | null)?.progress;
      if (p && typeof p === 'object') {
        const po = p as { value?: number; max?: number; label?: string };
        if (typeof po.value === 'number' && typeof po.max === 'number' && step.Seq >= latestSeq) {
          latestSeq = step.Seq;
          latest = { value: po.value, max: po.max, label: po.label };
        }
      }
    }
    return latest;
  }

  // Find the GraphStage for this spirit session
  const graphStage = computed<GraphStage | null>(() => {
    for (const [, gs] of activityStore.graphStages) {
      if (gs.SessionID === spiritSessionId.value) return gs;
    }
    return null;
  });

  // Get nodes for the current GraphStage.
  // Nodes may be embedded in gs.Nodes (initial REST payload) or stored
  // separately in graphNodes map (WS events / lazy fetch). Prefer the map.
  const nodes = computed<GraphNode[]>(() => {
    if (!graphStage.value) return [];
    const fromMap = activityStore.getGraphStageNodes(graphStage.value.ID);
    if (fromMap.length > 0) return fromMap;
    return graphStage.value.Nodes || [];
  });

  // Compute DAG layout positions
  const { layoutDAG } = usePlanDAGLayout<GraphNode>();
  const layoutResult = computed(() => {
    const list = nodes.value;
    if (list.length === 0) return { positions: new Map<string, { x: number; y: number }>(), computedWidth: 40 };
    return layoutDAG(list, DAG_OPTS);
  });

  // Convert GraphNode[] to Vue Flow node format
  const flowNodes = computed(() => {
    const nodeList = nodes.value;
    // Build a set of node IDs that have outgoing edges (i.e., are not exit nodes)
    const hasOutgoing = new Set<string>();
    for (const n of nodeList) {
      for (const depId of n.DependsOn || []) {
        hasOutgoing.add(depId);
      }
    }

    return nodeList.map((n) => {
      const pos = layoutResult.value.positions.get(n.ID) || { x: 0, y: 0 };
      const members = extractMembers(n);
      const durationMs = extractDuration(n);
      const error = extractError(n);
      const description = extractDescription(n);
      const textOutput = extractTextOutput(n);
      const isEntry = (n.DependsOn || []).length === 0;
      const isExit = !hasOutgoing.has(n.ID);

      return {
        id: n.ID,
        type: 'observe' as const,
        position: { x: pos.x, y: pos.y },
        data: {
          label: n.Label,
          dagNodeId: n.DagNodeID,
          teamStageId: n.TeamStageID,
          status: n.Status,
          dependsOn: n.DependsOn,
          members,
          activeMemberCount: members.filter((m) => m.status === 'running').length,
          mediaOutput: nodeOutputStore.getNodeOutput(n.TeamStageID || n.ID),
          progress: extractProgress(n.TeamStageID || n.ID),
          durationMs,
          error,
          description,
          textOutput,
          isEntry,
          isExit,
        },
      };
    });
  });

  // Convert DependsOn to Vue Flow edges
  const flowEdges = computed(() =>
    nodes.value.flatMap((n) =>
      (n.DependsOn || []).map((depId) => ({
        id: `${depId}→${n.ID}`,
        source: depId,
        target: n.ID,
        animated: true,
        type: 'smoothstep' as const,
      })),
    ),
  );

  /**
   * Highlight the dependency chain for a given node.
   * Returns sets of node IDs and edge IDs that form the chain
   * (upstream → current → downstream).
   */
  function getDependencyChain(nodeId: string): { nodeIds: Set<string>; edgeIds: Set<string> } {
    const nodeIds = new Set<string>();
    const edgeIds = new Set<string>();

    // Find the node
    const node = nodes.value.find((n) => n.ID === nodeId);
    if (!node) return { nodeIds, edgeIds };

    nodeIds.add(nodeId);

    // Upstream: all nodes this node depends on (transitive)
    const upstreamQueue = [...(node.DependsOn || [])];
    while (upstreamQueue.length > 0) {
      const depId = upstreamQueue.shift()!;
      if (nodeIds.has(depId)) continue;
      nodeIds.add(depId);
      edgeIds.add(`${depId}→${nodeId}`);
      const depNode = nodes.value.find((n) => n.ID === depId);
      if (depNode) {
        upstreamQueue.push(...(depNode.DependsOn || []));
      }
    }

    // Downstream: all nodes that depend on this node (transitive)
    const downstreamQueue = nodes.value.filter((n) => (n.DependsOn || []).includes(nodeId)).map((n) => n.ID);
    while (downstreamQueue.length > 0) {
      const childId = downstreamQueue.shift()!;
      if (nodeIds.has(childId)) continue;
      nodeIds.add(childId);
      edgeIds.add(`${nodeId}→${childId}`);
      const children = nodes.value.filter((n) => (n.DependsOn || []).includes(childId)).map((n) => n.ID);
      downstreamQueue.push(...children);
    }

    return { nodeIds, edgeIds };
  }

  return {
    graphStage,
    nodes,
    flowNodes,
    flowEdges,
    layoutResult,
    getDependencyChain,
  };
}
