import { shallowRef, triggerRef } from 'vue';
import { getSessionTree } from '../../session/api';
import type { SessionTreeNode } from '../../session/types';

/**
 * useSessionTree manages the spirit-session → recursive-tree cache used by
 * SessionTreeSidebar (Phase B-2). Trees are loaded lazily per spirit session
 * and cached; `findMemberSessionId` walks a cached tree to resolve a member's
 * agent session ID.
 */
export function useSessionTree() {
  const treesBySpirit = shallowRef<Map<string, SessionTreeNode>>(new Map());
  const spiritTreeNodes = shallowRef<SessionTreeNode[]>([]);

  async function loadTreeFor(spiritSessionId: string): Promise<SessionTreeNode | null> {
    const cached = treesBySpirit.value.get(spiritSessionId);
    if (cached) return cached;

    const tree = await getSessionTree(spiritSessionId);

    const nextMap = new Map(treesBySpirit.value);
    nextMap.set(spiritSessionId, tree);
    treesBySpirit.value = nextMap;
    triggerRef(treesBySpirit);

    const existingIdx = spiritTreeNodes.value.findIndex((n) => n.session.id === tree.session.id);
    if (existingIdx >= 0) {
      const nextNodes = spiritTreeNodes.value.slice();
      nextNodes[existingIdx] = tree;
      spiritTreeNodes.value = nextNodes;
    } else {
      spiritTreeNodes.value = [...spiritTreeNodes.value, tree];
    }
    triggerRef(spiritTreeNodes);

    return tree;
  }

  function findMemberSessionId(
    spiritSessionId: string,
    agentKey: string,
    teamSessionId?: string | null,
  ): string | null {
    const tree = treesBySpirit.value.get(spiritSessionId);
    if (!tree) return null;
    if (teamSessionId) {
      const teamNode = findTeamNode(tree, teamSessionId);
      if (teamNode) {
        const hit = walkForAgent(teamNode, agentKey);
        if (hit) return hit;
      }
    }
    return walkForAgent(tree, agentKey);
  }

  function findTeamNode(node: SessionTreeNode, teamSessionId: string): SessionTreeNode | null {
    if (node.session.id === teamSessionId) return node;
    for (const child of node.children) {
      const hit = findTeamNode(child, teamSessionId);
      if (hit) return hit;
    }
    return null;
  }

  function walkForAgent(node: SessionTreeNode, agentKey: string): string | null {
    if (node.session.member_agent_key === agentKey) return node.session.id;
    for (const child of node.children) {
      const hit = walkForAgent(child, agentKey);
      if (hit) return hit;
    }
    return null;
  }

  function clear(): void {
    treesBySpirit.value = new Map();
    spiritTreeNodes.value = [];
    triggerRef(treesBySpirit);
    triggerRef(spiritTreeNodes);
  }

  return {
    treesBySpirit,
    spiritTreeNodes,
    loadTreeFor,
    findMemberSessionId,
    clear,
  };
}
