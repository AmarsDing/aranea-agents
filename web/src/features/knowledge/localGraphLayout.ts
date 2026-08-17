// localGraphLayout（SP2 §SP2-8）：局部图谱轻量 2D 力导向布局（纯函数，≤200 节点）。
// B4：邻域 BFS 裁剪已移至服务端 ListDocumentNeighborhood（大库免全图传输），前端不再保留 bfsNeighborhood。
import type { CollectionGraphEdge, CollectionGraphNode } from './types';

export type LayoutPoint = { x: number; y: number };

/** 力导向布局：圆周初始化 + 固定迭代（斥力 n²、弹簧、中心引力）；结果确定性（无随机）。 */
export function layoutLocalGraph(
  nodes: CollectionGraphNode[],
  edges: CollectionGraphEdge[],
  width: number,
  height: number,
  iterations = 120,
): Map<string, LayoutPoint> {
  const n = nodes.length;
  const pos = new Map<string, LayoutPoint>();
  if (!n) return pos;
  const cx = width / 2;
  const cy = height / 2;
  const radius = Math.min(width, height) / 2 - 24;
  nodes.forEach((node, i) => {
    const a = (i / n) * Math.PI * 2;
    pos.set(node.doc_id, { x: cx + radius * Math.cos(a), y: cy + radius * Math.sin(a) });
  });

  const k = Math.sqrt((width * height) / Math.max(n, 1)) * 0.9;
  const ids = nodes.map((x) => x.doc_id);
  for (let iter = 0; iter < iterations; iter++) {
    const disp = new Map<string, LayoutPoint>(ids.map((id) => [id, { x: 0, y: 0 }]));
    // 斥力
    for (let i = 0; i < n; i++) {
      for (let j = i + 1; j < n; j++) {
        const a = pos.get(ids[i])!;
        const b = pos.get(ids[j])!;
        let dx = a.x - b.x;
        let dy = a.y - b.y;
        let dist = Math.hypot(dx, dy);
        if (dist < 0.01) {
          dx = 0.01;
          dy = 0.01;
          dist = 0.014;
        }
        const force = (k * k) / dist / dist;
        const fx = dx * force;
        const fy = dy * force;
        disp.get(ids[i])!.x += fx;
        disp.get(ids[i])!.y += fy;
        disp.get(ids[j])!.x -= fx;
        disp.get(ids[j])!.y -= fy;
      }
    }
    // 弹簧
    for (const e of edges) {
      const a = pos.get(e.source);
      const b = pos.get(e.target);
      if (!a || !b) continue;
      const dx = b.x - a.x;
      const dy = b.y - a.y;
      const dist = Math.max(Math.hypot(dx, dy), 0.01);
      const force = (dist - k) * 0.05;
      const fx = (dx / dist) * force;
      const fy = (dy / dist) * force;
      disp.get(e.source)!.x += fx;
      disp.get(e.source)!.y += fy;
      disp.get(e.target)!.x -= fx;
      disp.get(e.target)!.y -= fy;
    }
    // 中心引力 + 应用位移（退火）
    const cooling = 1 - iter / iterations;
    for (const id of ids) {
      const p = pos.get(id)!;
      const d = disp.get(id)!;
      d.x += (cx - p.x) * 0.02;
      d.y += (cy - p.y) * 0.02;
      p.x += d.x * cooling;
      p.y += d.y * cooling;
      p.x = Math.max(16, Math.min(width - 16, p.x));
      p.y = Math.max(16, Math.min(height - 16, p.y));
    }
  }
  return pos;
}
