import { ref, type Ref, type ComputedRef, toValue } from 'vue';
import { NODE_DEFAULT_WIDTH, NODE_DEFAULT_HEIGHT } from './types';

/** Minimal node shape for snap-guide calculations (avoids deep @vue-flow/core type instantiation). */
export interface SnapGuideNode {
  id: string;
  position: { x: number; y: number };
  dimensions?: { width?: number; height?: number };
}

interface SnapLine {
  orientation: 'horizontal' | 'vertical';
  position: number;
  from: number;
  to: number;
}

interface SnapResult {
  lines: SnapLine[];
  delta: { x: number; y: number };
}

const SNAP_THRESHOLD = 6;

interface NodeBounds {
  id: string;
  cx: number;
  cy: number;
  top: number;
  bottom: number;
  left: number;
  right: number;
}

function getNodesBounds(nodes: SnapGuideNode[]): NodeBounds[] {
  const result: NodeBounds[] = [];
  for (const n of nodes) {
    const dims = n.dimensions;
    const w = (dims?.width ?? NODE_DEFAULT_WIDTH) / 2;
    const h = (dims?.height ?? NODE_DEFAULT_HEIGHT) / 2;
    const cx = n.position.x + w;
    const cy = n.position.y + h;
    result.push({
      id: n.id,
      cx,
      cy,
      top: n.position.y,
      bottom: n.position.y + h * 2,
      left: n.position.x,
      right: n.position.x + w * 2,
    });
  }
  return result;
}

export function useSnapGuide(internalNodes: Ref<SnapGuideNode[]> | ComputedRef<SnapGuideNode[]>) {
  const snapLines = ref<SnapLine[]>([]);

  function computeSnapLines(draggedNodeIds: Set<string>): SnapResult {
    const allNodes = toValue(internalNodes);
    if (!allNodes || draggedNodeIds.size === 0) {
      snapLines.value = [];
      return { lines: [], delta: { x: 0, y: 0 } };
    }

    const dragged = allNodes.filter((n) => draggedNodeIds.has(n.id));
    const statics = allNodes.filter((n) => !draggedNodeIds.has(n.id));
    if (statics.length === 0) {
      snapLines.value = [];
      return { lines: [], delta: { x: 0, y: 0 } };
    }

    const draggedBounds = getNodesBounds(dragged);
    const staticBounds = getNodesBounds(statics);
    const lines: SnapLine[] = [];

    // 计算最佳吸附修正量：取距离最近的对齐偏移
    let bestDx = 0;
    let bestDy = 0;
    let bestDxScore = SNAP_THRESHOLD + 1;
    let bestDyScore = SNAP_THRESHOLD + 1;

    for (const db of draggedBounds) {
      for (const sb of staticBounds) {
        // 垂直方向对齐（cx / left / right）
        const vCandidates = [
          { delta: sb.cx - db.cx, pos: sb.cx, from: Math.min(db.top, sb.top), to: Math.max(db.bottom, sb.bottom) },
          {
            delta: sb.left - db.left,
            pos: sb.left,
            from: Math.min(db.top, sb.top),
            to: Math.max(db.bottom, sb.bottom),
          },
          {
            delta: sb.right - db.right,
            pos: sb.right,
            from: Math.min(db.top, sb.top),
            to: Math.max(db.bottom, sb.bottom),
          },
        ];
        for (const c of vCandidates) {
          if (Math.abs(c.delta) < SNAP_THRESHOLD) {
            lines.push({ orientation: 'vertical', position: c.pos, from: c.from, to: c.to });
            if (Math.abs(c.delta) < bestDxScore) {
              bestDxScore = Math.abs(c.delta);
              bestDx = c.delta;
            }
          }
        }

        // 水平方向对齐（cy / top / bottom）
        const hCandidates = [
          { delta: sb.cy - db.cy, pos: sb.cy, from: Math.min(db.left, sb.left), to: Math.max(db.right, sb.right) },
          { delta: sb.top - db.top, pos: sb.top, from: Math.min(db.left, sb.left), to: Math.max(db.right, sb.right) },
          {
            delta: sb.bottom - db.bottom,
            pos: sb.bottom,
            from: Math.min(db.left, sb.left),
            to: Math.max(db.right, sb.right),
          },
        ];
        for (const c of hCandidates) {
          if (Math.abs(c.delta) < SNAP_THRESHOLD) {
            lines.push({ orientation: 'horizontal', position: c.pos, from: c.from, to: c.to });
            if (Math.abs(c.delta) < bestDyScore) {
              bestDyScore = Math.abs(c.delta);
              bestDy = c.delta;
            }
          }
        }
      }
    }

    const seen = new Set<string>();
    const dedupedLines = lines.filter((l) => {
      const key = `${l.orientation}:${Math.round(l.position)}:${Math.round(l.from)}-${Math.round(l.to)}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });

    snapLines.value = dedupedLines;
    return {
      lines: dedupedLines,
      delta: { x: bestDx, y: bestDy },
    };
  }

  function clearSnapLines() {
    snapLines.value = [];
  }

  return {
    snapLines,
    computeSnapLines,
    clearSnapLines,
  };
}

export type { SnapLine, SnapResult };
