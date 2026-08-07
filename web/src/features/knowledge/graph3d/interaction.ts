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

/**
 * 交互状态机：hover(shown)/selected 分离。
 * - hover 优先于 selected（悬停时临时高亮一跳，移开后回落到选中锁定高亮）
 * - setHover 同值返回 false（去抖：防粒子相位重置/高亮闪刷）
 */
export class GraphInteraction {
  private hoverIndex: number | null = null;
  private selectedIndex: number | null = null;

  get hover(): number | null {
    return this.hoverIndex;
  }

  get selected(): number | null {
    return this.selectedIndex;
  }

  /** 有效高亮锚点：hover 优先，否则 selected。 */
  get active(): number | null {
    return this.hoverIndex ?? this.selectedIndex;
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
}
