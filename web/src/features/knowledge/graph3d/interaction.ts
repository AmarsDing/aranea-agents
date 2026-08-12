/**
 * interaction：G5-D 交互纯函数层（零 Vue/three 依赖，设计 §V12.8-1 D-1/D-3/D-4）。
 *
 * - oneHop：hover 一跳邻居全边表 O(E) 扫描（低频不建邻接表）
 * - GraphInteraction：shown(hover)/selected(单击锁定) 分离状态机，hover 优先
 * - wheelZoomFactor / isClickMovement：zoom-to-cursor 因子、点击/拖拽判别
 */

/** 单击/拖拽位移阈值（px，设计「位移 <5px 区分拖拽与点击」）。 */
export const CLICK_DRAG_THRESHOLD_PX = 5;

/** 位移 <5px 判定为点击。 */
export function isClickMovement(dx: number, dy: number): boolean {
  return dx * dx + dy * dy < CLICK_DRAG_THRESHOLD_PX * CLICK_DRAG_THRESHOLD_PX;
}

/** zoom-to-cursor 缩放因子：0.95^(-ΔY·0.01)（ΔY>0 缩小 → factor>1）。 */
export function wheelZoomFactor(deltaY: number): number {
  return Math.pow(0.95, -deltaY * 0.01);
}

export interface OneHopResult {
  /** 一跳邻居节点索引（含自身）。 */
  nodes: Set<number>;
  /** 关联边索引。 */
  edges: Set<number>;
}

/** 一跳邻居：全边表 O(E) 扫描（去抖后低频调用，不建邻接表）。 */
export function oneHop(edges: Int32Array, edgeCount: number, index: number): OneHopResult {
  const nodes = new Set<number>([index]);
  const hitEdges = new Set<number>();
  for (let e = 0; e < edgeCount; e++) {
    const a = edges[e * 2];
    const b = edges[e * 2 + 1];
    if (a === index || b === index) {
      hitEdges.add(e);
      nodes.add(a);
      nodes.add(b);
    }
  }
  return { nodes, edges: hitEdges };
}

/** M4：BFS N 跳邻居集（聚焦模式）。hops=0 仅根；边集 = 两端点都在节点集内的边。 */
export function nHop(
  edges: Int32Array,
  edgeCount: number,
  root: number,
  hops: number,
): { nodes: Set<number>; edges: Set<number> } {
  const nodes = new Set<number>([root]);
  if (hops > 0) {
    let frontier = [root];
    for (let h = 0; h < hops; h++) {
      const next: number[] = [];
      for (let e = 0; e < edgeCount; e++) {
        const a = edges[e * 2];
        const b = edges[e * 2 + 1];
        if (frontier.includes(a) && !nodes.has(b)) {
          nodes.add(b);
          next.push(b);
        } else if (frontier.includes(b) && !nodes.has(a)) {
          nodes.add(a);
          next.push(a);
        }
      }
      frontier = next;
      if (frontier.length === 0) break;
    }
  }
  const edgeSet = new Set<number>();
  for (let e = 0; e < edgeCount; e++) {
    if (nodes.has(edges[e * 2]) && nodes.has(edges[e * 2 + 1])) edgeSet.add(e);
  }
  return { nodes, edges: edgeSet };
}

/**
 * 交互状态机：hover(shown)/selected 分离。
 * - hover 优先于 selected（悬停时临时高亮一跳，移开后回落到选中锁定高亮）
 * - setHover 同值返回 false（去抖：防粒子相位重置/高亮闪刷）
 */
export class GraphInteraction {
  private hoverIndex: number | null = null;
  private selectedIndex: number | null = null;
  /** M4 聚焦锁定：非 null 时 active 由 focused 驱动（hover 不覆盖）。 */
  private _focused: number | null = null;
  private _focusHops = 2;

  get hover(): number | null {
    return this.hoverIndex;
  }

  get selected(): number | null {
    return this.selectedIndex;
  }

  get focused(): number | null {
    return this._focused;
  }

  get focusHops(): number {
    return this._focusHops;
  }

  /** 有效高亮锚点：聚焦锁定 > hover > selected。 */
  get active(): number | null {
    return this._focused ?? this.hoverIndex ?? this.selectedIndex;
  }

  /** hover 变更；返回是否变化。 */
  setHover(i: number | null): boolean {
    if (this.hoverIndex === i) return false;
    this.hoverIndex = i;
    return true;
  }

  /** 选中变更；返回是否变化。 */
  setSelected(i: number | null): boolean {
    if (this.selectedIndex === i) return false;
    this.selectedIndex = i;
    return true;
  }

  /** M4：聚焦锁定（单击节点触发，BFS N 跳 dim）。 */
  setFocus(index: number, hops: number): void {
    this._focused = index;
    this._focusHops = hops;
  }

  /** M4：解除聚焦锁定（单击空白触发），恢复 hover 驱动。 */
  clearFocus(): void {
    this._focused = null;
  }
}
