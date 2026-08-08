/**
 * pickMath：G5 渲染管线 v2 自研射线拾取纯数学（可单测）。
 *
 * 替代 three InstancedMesh.raycast（每实例一次矩阵求逆 = 万级 hover 卡顿元凶）：
 * 射线-球纯循环 O(N)，无矩阵运算；阈值 = max(节点半径, t·每像素世界尺寸·slackPx)
 * （远距离节点屏幕占比小，slack 随距离放大保证可点）；最近 t 获胜（遮挡语义）。
 */

/** 拾取宽容像素数（半径外的 slack 圈宽度）。 */
export const PICK_SLACK_PX = 4;

/**
 * 射线-球最近命中。
 *
 * @param positions 节点位置（3N 紧凑）
 * @param sizes     节点半径（N）
 * @param count     节点数
 * @param ox/oy/oz  射线原点（相机世界坐标）
 * @param dx/dy/dz  射线方向（单位向量）
 * @param worldPerPixel 距相机 1 单位处每像素的世界尺寸 = 2·tan(fov/2)/viewportHeightPx
 * @returns 命中节点索引；未命中 -1
 */
export function pickNode(
  positions: Float32Array,
  sizes: Float32Array,
  count: number,
  ox: number,
  oy: number,
  oz: number,
  dx: number,
  dy: number,
  dz: number,
  worldPerPixel: number,
): number {
  let best = -1;
  let bestT = Infinity;
  for (let i = 0; i < count; i++) {
    const px = positions[i * 3] - ox;
    const py = positions[i * 3 + 1] - oy;
    const pz = positions[i * 3 + 2] - oz;
    const t = px * dx + py * dy + pz * dz;
    if (t <= 0) continue; // 相机背后
    const cx = ox + dx * t;
    const cy = oy + dy * t;
    const cz = oz + dz * t;
    const ex = positions[i * 3] - cx;
    const ey = positions[i * 3 + 1] - cy;
    const ez = positions[i * 3 + 2] - cz;
    const d2 = ex * ex + ey * ey + ez * ez;
    const slack = worldPerPixel * PICK_SLACK_PX * t;
    const thr = sizes[i] > slack ? sizes[i] : slack;
    if (d2 <= thr * thr && t < bestT) {
      bestT = t;
      best = i;
    }
  }
  return best;
}
