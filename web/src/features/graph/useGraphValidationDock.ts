// web/src/features/graph/useGraphValidationDock.ts
import { computed, onMounted, onUnmounted, ref } from 'vue';
import type { Ref } from 'vue';
import type { ValidationIssue, NodeIssueInfo } from './types';
import { pickNodeIssueMap } from './validationIssues';

/**
 * R2-7 页面级校验 dock 状态管理：
 * - 底部校验面板开合 + 级别计数
 * - 画布聚光灯（locate → spotlightNodeId，其余节点压暗）
 * - Esc 分层清除：先清聚光灯，再关面板
 */
export function useGraphValidationDock(
  issues: Ref<ValidationIssue[]>,
  opts: { onRevalidate?: () => Promise<void> | void } = {},
) {
  const panelOpen = ref(false);
  const spotlightNodeId = ref<string | null>(null);
  const validating = ref(false);

  const errorCount = computed(() => issues.value.filter((i) => i.level === 'error').length);
  const warningCount = computed(() => issues.value.filter((i) => i.level === 'warning').length);
  const nodeIssueMap = computed<Record<string, ValidationIssue>>(() => pickNodeIssueMap(issues.value));

  function clearSpotlight() {
    spotlightNodeId.value = null;
  }

  function openPanel() {
    panelOpen.value = true;
  }

  function closePanel() {
    panelOpen.value = false;
    clearSpotlight();
  }

  function togglePanel() {
    if (panelOpen.value) {
      closePanel();
    } else {
      openPanel();
    }
  }

  function locateNode(nodeId: string) {
    spotlightNodeId.value = nodeId;
  }

  async function revalidate() {
    if (!opts.onRevalidate) return;
    validating.value = true;
    try {
      await opts.onRevalidate();
    } finally {
      validating.value = false;
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key !== 'Escape') return;
    if (spotlightNodeId.value) {
      clearSpotlight();
    } else if (panelOpen.value) {
      closePanel();
    } else {
      return;
    }
    e.stopPropagation();
  }

  onMounted(() => window.addEventListener('keydown', onKeydown));
  onUnmounted(() => window.removeEventListener('keydown', onKeydown));

  return {
    panelOpen,
    spotlightNodeId,
    validating,
    errorCount,
    warningCount,
    nodeIssueMap: nodeIssueMap as Ref<Record<string, NodeIssueInfo>>,
    openPanel,
    closePanel,
    togglePanel,
    locateNode,
    clearSpotlight,
    revalidate,
  };
}
