// V2 ParticleField 纯函数层（方案 §三-V2）：星光闪烁 / 流星 / 视差双层。
// 纯函数抽出以便单测（jsdom 无 Canvas2D 上下文，绘制路径不可测）。

export interface FieldParticle {
  x: number;
  y: number;
  vx: number;
  vy: number;
  r: number;
  hue: number;
  /** twinkle 相位（弧度，随机） */
  phase: number;
  /** 视差深度：0.35 远层（小/慢）| 1 近层（大/快） */
  depth: number;
  /** 帧内缓存：视差后的绘制坐标（近距连线用），由渲染循环写入 */
  px: number;
  py: number;
}

export interface Meteor {
  /** 头部出生点（屏上缘外） */
  x: number;
  y: number;
  /** 归一化方向（斜向坠入，dy > 0） */
  dx: number;
  dy: number;
  /** 出生时间戳 ms（rAF 时间轴） */
  born: number;
  /** 生命周期 ms */
  life: number;
  /** 尾迹长度 px */
  length: number;
  /** 全程位移 px */
  distance: number;
}

export const METEOR_MIN_INTERVAL = 4000;
export const METEOR_MAX_INTERVAL = 8000;
/** 流星生命周期（方案：短促划破感，300ms） */
export const METEOR_LIFE = 300;
/** 视差满幅位移 px（近层 depth=1 时） */
export const PARALLAX_MAX_SHIFT = 18;

const TWINKLE_BASE = 0.55;
const TWINKLE_AMP = 0.3;
const TWINKLE_SPEED = 0.0016; // rad/ms ≈ 3.9s 周期

/** 星光闪烁透明度：正弦振荡，输出 [0.25, 0.85]。 */
export function twinkleAlpha(phase: number, t: number): number {
  return TWINKLE_BASE + TWINKLE_AMP * Math.sin(t * TWINKLE_SPEED + phase);
}

/** 下一颗流星间隔 ms：[4s, 8s]。 */
export function nextMeteorDelay(rng: () => number = Math.random): number {
  return METEOR_MIN_INTERVAL + rng() * (METEOR_MAX_INTERVAL - METEOR_MIN_INTERVAL);
}

/** 生成流星：屏上缘外随机位置，斜向坠入（左右各半）。 */
export function createMeteor(width: number, now: number, rng: () => number = Math.random): Meteor {
  const dirSign = rng() < 0.5 ? -1 : 1;
  const slope = 0.5 + rng() * 0.5; // 方向斜率 0.5~1（偏陡的下坠）
  const norm = Math.hypot(1, slope);
  return {
    x: rng() * width,
    y: -20,
    dx: dirSign / norm,
    dy: slope / norm,
    born: now,
    life: METEOR_LIFE,
    length: 90 + rng() * 60,
    distance: 260 + rng() * 220,
  };
}

/** 流星进度：出生 0 → 寿终 1；>=1 应回收。 */
export function meteorProgress(m: Meteor, now: number): number {
  return (now - m.born) / m.life;
}

/** 流星头部当前位置（按进度沿方向位移）。 */
export function meteorHead(m: Meteor, now: number): { x: number; y: number } {
  const p = meteorProgress(m, now);
  return { x: m.x + m.dx * m.distance * p, y: m.y + m.dy * m.distance * p };
}

/**
 * 视差偏移：鼠标相对屏心的归一化位移 × 深度系数（反向，营造景深）。
 * 鼠标越界（离开画布的大负值）钳位到 ±1，防止偏移爆量。
 */
export function parallaxOffset(
  mouseX: number,
  mouseY: number,
  width: number,
  height: number,
  depth: number,
): { x: number; y: number } {
  if (width <= 0 || height <= 0) return { x: 0, y: 0 };
  const clamp = (v: number) => Math.max(-1, Math.min(1, v));
  const nx = clamp((mouseX - width / 2) / (width / 2));
  const ny = clamp((mouseY - height / 2) / (height / 2));
  // +0：消除 -0（屏心时 -0 * depth === -0，语义应为 0）
  return { x: -nx * PARALLAX_MAX_SHIFT * depth + 0, y: -ny * PARALLAX_MAX_SHIFT * depth + 0 };
}

/** 视差双层种子：55% 远层（depth 0.35，更小更慢）+ 45% 近层（depth 1）。 */
export function seedField(
  width: number,
  height: number,
  budget: number,
  rng: () => number = Math.random,
): FieldParticle[] {
  return Array.from({ length: budget }, () => {
    const far = rng() < 0.55;
    const speedScale = far ? 0.5 : 1;
    const x = rng() * width;
    const y = rng() * height;
    return {
      x,
      y,
      vx: (rng() - 0.5) * 0.22 * speedScale,
      vy: (rng() - 0.5) * 0.22 * speedScale,
      r: (0.8 + rng() * 1.6) * (far ? 0.6 : 1),
      hue: rng() < 0.72 ? 197 : 262, // cyan / violet
      phase: rng() * Math.PI * 2,
      depth: far ? 0.35 : 1,
      px: x,
      py: y,
    };
  });
}
