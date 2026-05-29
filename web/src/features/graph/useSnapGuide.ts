import { ref, type Ref, type ComputedRef, toValue } from "vue";
import type { Node } from "@vue-flow/core";
import { NODE_DEFAULT_WIDTH, NODE_DEFAULT_HEIGHT } from "./types";

interface SnapLine {
  orientation: "horizontal" | "vertical";
  position: number;
  from: number;
  to: number;
}

const SNAP_THRESHOLD = 6;

function getNodesBounds(nodes: Node[]) {
  const result: { id: string; cx: number; cy: number; top: number; bottom: number; left: number; right: number }[] = [];
  for (const n of nodes) {
    const w = (n.dimensions?.width ?? NODE_DEFAULT_WIDTH) / 2;
    const h = (n.dimensions?.height ?? NODE_DEFAULT_HEIGHT) / 2;
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

export function useSnapGuide(
  internalNodes: Ref<Node[]> | ComputedRef<Node[]>,
) {
  const snapLines = ref<SnapLine[]>([]);

  function computeSnapLines(draggedNodeIds: Set<string>) {
    const allNodes = toValue(internalNodes);
    if (!allNodes || draggedNodeIds.size === 0) {
      snapLines.value = [];
      return;
    }

    const dragged = allNodes.filter((n) => draggedNodeIds.has(n.id));
    const statics = allNodes.filter((n) => !draggedNodeIds.has(n.id));
    if (statics.length === 0) {
      snapLines.value = [];
      return;
    }

    const draggedBounds = getNodesBounds(dragged);
    const staticBounds = getNodesBounds(statics);
    const lines: SnapLine[] = [];

    for (const db of draggedBounds) {
      for (const sb of staticBounds) {
        if (Math.abs(db.cx - sb.cx) < SNAP_THRESHOLD) {
          lines.push({
            orientation: "vertical",
            position: sb.cx,
            from: Math.min(db.top, sb.top),
            to: Math.max(db.bottom, sb.bottom),
          });
        }
        if (Math.abs(db.left - sb.left) < SNAP_THRESHOLD) {
          lines.push({
            orientation: "vertical",
            position: sb.left,
            from: Math.min(db.top, sb.top),
            to: Math.max(db.bottom, sb.bottom),
          });
        }
        if (Math.abs(db.right - sb.right) < SNAP_THRESHOLD) {
          lines.push({
            orientation: "vertical",
            position: sb.right,
            from: Math.min(db.top, sb.top),
            to: Math.max(db.bottom, sb.bottom),
          });
        }
        if (Math.abs(db.cy - sb.cy) < SNAP_THRESHOLD) {
          lines.push({
            orientation: "horizontal",
            position: sb.cy,
            from: Math.min(db.left, sb.left),
            to: Math.max(db.right, sb.right),
          });
        }
        if (Math.abs(db.top - sb.top) < SNAP_THRESHOLD) {
          lines.push({
            orientation: "horizontal",
            position: sb.top,
            from: Math.min(db.left, sb.left),
            to: Math.max(db.right, sb.right),
          });
        }
        if (Math.abs(db.bottom - sb.bottom) < SNAP_THRESHOLD) {
          lines.push({
            orientation: "horizontal",
            position: sb.bottom,
            from: Math.min(db.left, sb.left),
            to: Math.max(db.right, sb.right),
          });
        }
      }
    }

    const seen = new Set<string>();
    snapLines.value = lines.filter((l) => {
      const key = `${l.orientation}:${Math.round(l.position)}:${Math.round(l.from)}-${Math.round(l.to)}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
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

export type { SnapLine };
