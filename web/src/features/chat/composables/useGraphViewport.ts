// web/src/features/chat/composables/useGraphViewport.ts
//
// Graph 画布缩放/平移视口状态（chat GraphStageBlock 专用）。
// - 缩放：按钮步进 / 滚轮以光标为中心缩放，范围 [MIN_SCALE, MAX_SCALE]
// - 平移：左键拖拽（阈值 3px 内视为点击，不触发 pan）
// - zoomFit：内容缩放到视口内并居中（只缩不放，scale ≤ 1）
import { computed, ref, type ComputedRef, type Ref } from 'vue';

export const GRAPH_VIEWPORT_MIN_SCALE = 0.4;
export const GRAPH_VIEWPORT_MAX_SCALE = 2;
export const GRAPH_VIEWPORT_ZOOM_STEP = 1.15;
/** 拖拽位移阈值（px）：超过才算 pan，否则保留为点击。 */
export const GRAPH_VIEWPORT_PAN_THRESHOLD = 3;

export interface GraphViewport {
  scale: Ref<number>;
  tx: Ref<number>;
  ty: Ref<number>;
  isPanning: Ref<boolean>;
  /** 最近一次交互为拖拽 pan（用于抑制节点 click）。 */
  justPanned: Ref<boolean>;
  transformStyle: ComputedRef<{ transform: string; transformOrigin: string }>;
  /** 以视口坐标 (mx, my) 为中心缩放（光标/视口中心锚点）。 */
  zoomAt: (mx: number, my: number, next: number) => void;
  zoomIn: () => void;
  zoomOut: () => void;
  reset: () => void;
  zoomFit: (contentW: number, contentH: number, viewportW: number, viewportH: number) => void;
  onWheel: (e: WheelEvent) => void;
  onPanStart: (e: PointerEvent) => void;
  onPanMove: (e: PointerEvent) => void;
  onPanEnd: () => void;
}

function clampScale(s: number): number {
  return Math.min(GRAPH_VIEWPORT_MAX_SCALE, Math.max(GRAPH_VIEWPORT_MIN_SCALE, s));
}

export function useGraphViewport(): GraphViewport {
  const scale = ref(1);
  const tx = ref(0);
  const ty = ref(0);
  const isPanning = ref(false);
  const justPanned = ref(false);

  const transformStyle = computed(() => ({
    transform: `translate(${tx.value}px, ${ty.value}px) scale(${scale.value})`,
    transformOrigin: '0 0',
  }));

  /** 以视口坐标 (mx, my) 为中心缩放：缩放前后光标下的内容点保持不动。 */
  function zoomAt(mx: number, my: number, next: number) {
    const prev = scale.value;
    const s = clampScale(next);
    if (s === prev) return;
    tx.value = mx - ((mx - tx.value) * s) / prev;
    ty.value = my - ((my - ty.value) * s) / prev;
    scale.value = s;
  }

  function zoomIn() {
    zoomAt(0, 0, scale.value * GRAPH_VIEWPORT_ZOOM_STEP);
    // 按钮缩放以视口左上角为锚不够直观，但 tx/ty 修正需要视口尺寸；
    // 简化：按钮缩放围绕当前 translate 原点，用户可再拖拽调整。
  }

  function zoomOut() {
    zoomAt(0, 0, scale.value / GRAPH_VIEWPORT_ZOOM_STEP);
  }

  function reset() {
    scale.value = 1;
    tx.value = 0;
    ty.value = 0;
  }

  function zoomFit(contentW: number, contentH: number, viewportW: number, viewportH: number) {
    if (contentW <= 0 || contentH <= 0 || viewportW <= 0 || viewportH <= 0) return;
    const s = clampScale(Math.min(viewportW / contentW, viewportH / contentH, 1));
    scale.value = s;
    tx.value = (viewportW - contentW * s) / 2;
    ty.value = (viewportH - contentH * s) / 2;
  }

  function onWheel(e: WheelEvent) {
    e.preventDefault();
    const el = e.currentTarget as HTMLElement | null;
    const rect = el?.getBoundingClientRect();
    const mx = rect ? e.clientX - rect.left : 0;
    const my = rect ? e.clientY - rect.top : 0;
    const next = e.deltaY < 0 ? scale.value * GRAPH_VIEWPORT_ZOOM_STEP : scale.value / GRAPH_VIEWPORT_ZOOM_STEP;
    zoomAt(mx, my, next);
  }

  // ── 拖拽平移 ──
  let panStartX = 0;
  let panStartY = 0;
  let panBaseTx = 0;
  let panBaseTy = 0;
  let panActive = false;

  function onPanStart(e: PointerEvent) {
    if (e.button !== 0) return;
    panActive = true;
    isPanning.value = true;
    justPanned.value = false;
    panStartX = e.clientX;
    panStartY = e.clientY;
    panBaseTx = tx.value;
    panBaseTy = ty.value;
  }

  function onPanMove(e: PointerEvent) {
    if (!panActive) return;
    const dx = e.clientX - panStartX;
    const dy = e.clientY - panStartY;
    if (!justPanned.value && Math.hypot(dx, dy) < GRAPH_VIEWPORT_PAN_THRESHOLD) return;
    justPanned.value = true;
    tx.value = panBaseTx + dx;
    ty.value = panBaseTy + dy;
  }

  function onPanEnd() {
    panActive = false;
    isPanning.value = false;
  }

  return {
    scale,
    tx,
    ty,
    isPanning,
    justPanned,
    transformStyle,
    zoomAt,
    zoomIn,
    zoomOut,
    reset,
    zoomFit,
    onWheel,
    onPanStart,
    onPanMove,
    onPanEnd,
  };
}
