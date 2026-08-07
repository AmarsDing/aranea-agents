/**
 * particleMath：G5 深空图谱粒子流纯数学（fast-graph 1:1 复刻，设计 §V12.8-1）。
 *
 * - MAX=80 并发上限；SPEED=0.45/s（≈2.2s/边）
 * - 相位均布 prog[i]=i/n（连续流观感）
 * - easeInOutQuad 缓动（加速→减速的数据流感）
 * - 时变彩虹：hue=0.5+0.32·sin((t·0.6+p·2.2+i·0.12)·π)，sat=0.9，light=0.62
 */

export const PARTICLE_MAX = 80;
export const PARTICLE_SPEED = 0.45;

/** 相位均布：prog[i]=i/n。 */
export function spreadPhases(count: number): Float32Array {
  const out = new Float32Array(count);
  for (let i = 0; i < count; i++) out[i] = i / count;
  return out;
}

/** 相位推进（对 1 回绕）。 */
export function advancePhase(prog: number, dt: number): number {
  return (prog + dt * PARTICLE_SPEED) % 1;
}

/** ease-in-out 二次缓动。 */
export function easeInOutQuad(p: number): number {
  return p < 0.5 ? 2 * p * p : 1 - Math.pow(-2 * p + 2, 2) / 2;
}

/** 时变彩虹色（hue 在 0.18~0.82 间循环：青→蓝→紫→品红）。 */
export function particleHsl(time: number, phase: number, index: number): { h: number; s: number; l: number } {
  const hue = 0.5 + 0.32 * Math.sin((time * 0.6 + phase * 2.2 + index * 0.12) * Math.PI);
  return { h: hue, s: 0.9, l: 0.62 };
}

/** 粒子位置：src→dst 按 ease 插值，写入 out[offset..offset+2]。 */
export function particlePosition(
  src: readonly [number, number, number] | number[],
  dst: readonly [number, number, number] | number[],
  ease: number,
  out: Float32Array,
  offset: number,
): void {
  out[offset] = src[0] + (dst[0] - src[0]) * ease;
  out[offset + 1] = src[1] + (dst[1] - src[1]) * ease;
  out[offset + 2] = src[2] + (dst[2] - src[2]) * ease;
}
