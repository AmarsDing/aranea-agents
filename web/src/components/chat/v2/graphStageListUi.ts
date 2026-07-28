// web/src/components/chat/v2/graphStageListUi.ts
//
// GraphStageList（GraphStageBlock 移动降级视图）的纯函数：容器状态聚合、
// DAG 拓扑排序、节点默认展开判定。与组件解耦便于单测。

/** 终态优先后端；运行中由子节点聚合（与 GraphStageBlock 的 derivedStatus 同规则）。 */
export function deriveGraphStageStatus(backendStatus: string, nodes: readonly { Status: string }[]): string {
  if (backendStatus === 'completed' || backendStatus === 'failed' || backendStatus === 'interrupted') {
    return backendStatus;
  }
  if (nodes.length === 0) return backendStatus || 'running';
  const hasFailed = nodes.some((n) => n.Status === 'failed');
  const hasInterrupted = nodes.some((n) => n.Status === 'interrupted');
  if (nodes.every((n) => n.Status === 'completed')) return 'completed';
  if (hasFailed) return 'failed';
  if (hasInterrupted) return 'interrupted';
  return 'running';
}

/**
 * 移动列表按 DAG 拓扑序渲染：层（longest-path layering）升序 → 层内序升序。
 * 不在布局映射中的节点（异常数据）追加到末尾并保持输入顺序；不修改入参。
 */
export function orderGraphNodesForList<T extends { ID: string }>(
  nodes: readonly T[],
  layers: ReadonlyMap<string, number>,
  orderInLayer: ReadonlyMap<string, number>,
): T[] {
  return [...nodes].sort((a, b) => {
    const la = layers.get(a.ID);
    const lb = layers.get(b.ID);
    if (la === undefined && lb === undefined) return 0;
    if (la === undefined) return 1;
    if (lb === undefined) return -1;
    if (la !== lb) return la - lb;
    return (orderInLayer.get(a.ID) ?? 0) - (orderInLayer.get(b.ID) ?? 0);
  });
}

/** 默认展开需要关注的节点（进行中/失败/中断），其余收起保持列表紧凑。 */
export function defaultGraphNodeExpanded(status: string): boolean {
  return status === 'running' || status === 'failed' || status === 'interrupted';
}
