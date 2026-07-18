// web/src/features/chat/composables/useObserveGraph.ts
import { computed, type Ref } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import { usePlanDAGLayout } from './usePlanDAGLayout';
import { useNodeOutputStore } from '../../../stores/chat/nodeOutputStore';
import type { GraphStage, GraphNode } from '../v2Types';

const DAG_OPTS = {
  width: 800,
  nodeWidth: 200,
  nodeHeight: 140,
  gapX: 40,
  gapY: 60,
  padX: 20,
};

/**
 * useObserveGraph transforms activityV2Store data into a format suitable
 * for the ObservationCanvas (Vue Flow). It extracts the GraphStage and its
 * nodes, then computes layout positions using the existing DAG layout algorithm.
 */
export function useObserveGraph(spiritSessionId: Ref<string>) {
  const activityStore = useChatActivityStore();
  const nodeOutputStore = useNodeOutputStore();

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

  // Get nodes for the current GraphStage
  const nodes = computed<GraphNode[]>(() => {
    if (!graphStage.value) return [];
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
  const flowNodes = computed(() =>
    nodes.value.map((n) => {
      const pos = layoutResult.value.positions.get(n.ID) || { x: 0, y: 0 };
      return {
        id: n.ID,
        type: 'observe',
        position: { x: pos.x, y: pos.y },
        data: {
          label: n.Label,
          dagNodeId: n.DagNodeID,
          teamStageId: n.TeamStageID,
          status: n.Status,
          dependsOn: n.DependsOn,
          mediaOutput: nodeOutputStore.getNodeOutput(n.TeamStageID || n.ID),
          progress: extractProgress(n.TeamStageID || n.ID),
        },
      };
    }),
  );

  // Convert DependsOn to Vue Flow edges
  const flowEdges = computed(() =>
    nodes.value.flatMap((n) =>
      (n.DependsOn || []).map((depId) => ({
        id: `${depId}→${n.ID}`,
        source: depId,
        target: n.ID,
        animated: true,
      })),
    ),
  );

  return {
    graphStage,
    nodes,
    flowNodes,
    flowEdges,
    layoutResult,
  };
}
