/**
 * qualityTiers：G5 渲染管线 v2 自适应画质分级（纯 TS，可单测）。
 *
 * - 初始分级按节点数（万级直接 LOW 起步，保交互流畅）
 * - 运行期 governor：FPS 滑动窗连续低帧降档、连续高帧升档（不超初始档顶，防振荡）
 * - 档规格驱动渲染参数：bloom 开关/分辨率、pixelRatio 上限、标签候选数
 */

export const QUALITY_HIGH = 0;
export const QUALITY_MED = 1;
export const QUALITY_LOW = 2;
export type QualityTier = typeof QUALITY_HIGH | typeof QUALITY_MED | typeof QUALITY_LOW;

export interface QualitySpec {
  /** 是否启用 UnrealBloom 后处理。 */
  bloom: boolean;
  /** bloom 分辨率相对画布比例（降档省 GPU）。 */
  bloomScale: number;
  /** renderer pixelRatio 上限。 */
  maxPixelRatio: number;
  /** 标签候选池上限（degree top-K）。 */
  labelCandidates: number;
  /** HUD 指示短名。 */
  label: 'HIGH' | 'MID' | 'LOW';
}

export const QUALITY_SPECS: readonly QualitySpec[] = [
  // v3 可读性：标签候选 200/100/40 → 80/48/24（同屏标签减半，缓解密集区叠字）
  { bloom: true, bloomScale: 0.5, maxPixelRatio: 2, labelCandidates: 80, label: 'HIGH' },
  { bloom: true, bloomScale: 0.34, maxPixelRatio: 1.5, labelCandidates: 48, label: 'MID' },
  { bloom: false, bloomScale: 0.25, maxPixelRatio: 1, labelCandidates: 24, label: 'LOW' },
] as const;

/** 初始分级阈值（节点数）。 */
export const TIER_MED_MIN_NODES = 2500;
export const TIER_LOW_MIN_NODES = 8000;

/** governor 阈值：连续 DOWN_FRAMES 帧低于 DOWN_FPS → 降档；连续 UP_FRAMES 帧高于 UP_FPS → 升档。 */
export const GOVERN_DOWN_FPS = 45;
export const GOVERN_UP_FPS = 57;
export const GOVERN_DOWN_FRAMES = 90;
export const GOVERN_UP_FRAMES = 600;

/** 初始分级：节点数越大起步档越低（万级 LOW 保帧率）。 */
export function initialTier(nodeCount: number): QualityTier {
  if (nodeCount >= TIER_LOW_MIN_NODES) return QUALITY_LOW;
  if (nodeCount >= TIER_MED_MIN_NODES) return QUALITY_MED;
  return QUALITY_HIGH;
}

/**
 * 运行期画质决策（纯函数）：
 * - 连续低帧 ≥ DOWN_FRAMES → 降一档（不超低档 LOW）
 * - 连续高帧 ≥ UP_FRAMES → 升一档（不超初始档顶 ceiling，防与初始分级打架振荡）
 * - 降档优先（两计数理论互斥，防御同真时先保命）
 */
export function governTier(
  current: QualityTier,
  lowFrames: number,
  highFrames: number,
  ceiling: QualityTier,
): QualityTier {
  if (lowFrames >= GOVERN_DOWN_FRAMES) {
    return Math.min(current + 1, QUALITY_LOW) as QualityTier;
  }
  if (highFrames >= GOVERN_UP_FRAMES) {
    return Math.max(current - 1, ceiling) as QualityTier;
  }
  return current;
}
