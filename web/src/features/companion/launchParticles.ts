/**
 * 确认通过后的粒子发射开启动画（M74 V2-T5，需求 §2.5「确认卡化作粒子流发射」）。
 *
 * `makeBurstParticles` 为纯函数（注入 rng 可单测）；`spawnLaunchBurst` 为
 * DOM/WAAPI 副作用层（不可单测，保持极薄）。
 */

export type BurstParticle = {
  /** 发射角（度，-180..0 向上半球，0 = 正右）。 */
  angleDeg: number;
  /** 飞行距离（px）。 */
  distance: number;
  /** 动画时长（ms）。 */
  durationMs: number;
  /** 起始延迟（ms）。 */
  delayMs: number;
  /** 粒子直径（px）。 */
  size: number;
};

export const BURST_PARTICLE_COUNT = 36;

/** 生成一波发射粒子参数：向上半球散射，近快远慢，整体 <1s 收场。 */
export function makeBurstParticles(count: number = BURST_PARTICLE_COUNT, rng: () => number = Math.random): BurstParticle[] {
  const out: BurstParticle[] = [];
  for (let i = 0; i < count; i++) {
    out.push({
      angleDeg: -180 + rng() * 180,
      distance: 41 + rng() * 179, // (40, 220]
      durationMs: 450 + rng() * 500, // [450, 950]
      delayMs: rng() * 120,
      size: 2 + rng() * 4, // [2, 6]
    });
  }
  return out;
}

/**
 * 在 host 内 origin 处发射一波粒子（WAAPI，结束自动清理）。
 * host 需 position:relative/absolute；origin 缺省取 host 中心。
 */
export function spawnLaunchBurst(host: HTMLElement, origin?: { x: number; y: number }): void {
  const rect = host.getBoundingClientRect();
  const cx = origin?.x ?? rect.width / 2;
  const cy = origin?.y ?? rect.height / 2;
  const layer = document.createElement('div');
  layer.className = 'launch-burst';
  layer.setAttribute('aria-hidden', 'true');
  host.appendChild(layer);

  let maxEnd = 0;
  for (const p of makeBurstParticles()) {
    const rad = (p.angleDeg * Math.PI) / 180;
    const dx = Math.cos(rad) * p.distance;
    const dy = Math.sin(rad) * p.distance;
    const el = document.createElement('i');
    el.className = 'launch-burst__p';
    el.style.left = `${cx}px`;
    el.style.top = `${cy}px`;
    el.style.width = `${p.size}px`;
    el.style.height = `${p.size}px`;
    layer.appendChild(el);
    el.animate(
      [
        { transform: 'translate(-50%, -50%) scale(1)', opacity: 1 },
        { transform: `translate(calc(-50% + ${dx}px), calc(-50% + ${dy}px)) scale(0.2)`, opacity: 0 },
      ],
      { duration: p.durationMs, delay: p.delayMs, easing: 'cubic-bezier(0.1, 0.7, 0.3, 1)', fill: 'forwards' },
    );
    maxEnd = Math.max(maxEnd, p.delayMs + p.durationMs);
  }
  window.setTimeout(() => layer.remove(), maxEnd + 60);
}
