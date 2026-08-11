<template>
  <canvas v-if="enabled" ref="canvasRef" class="kb-particle-field" aria-hidden="true" />
</template>

<script setup lang="ts">
// 深空粒子背景（V2 升级，方案 §三-V2）：视差双层漂浮粒子 + 星光闪烁 + 流星 + 鼠标斥力 + 近距连线。
// 纯函数层在 features/knowledge/particles.ts（twinkle/meteor/parallax/seed 可单测）。
// 降级：reduced-motion 或低端设备（particleBudget()=0）时不渲染；页面不可见时停帧。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { particleBudget, useReducedMotion } from '../../../features/knowledge/useReducedMotion';
import {
  createMeteor,
  meteorHead,
  meteorProgress,
  nextMeteorDelay,
  parallaxOffset,
  seedField,
  twinkleAlpha,
  type FieldParticle,
  type Meteor,
} from '../../../features/knowledge/particles';

const canvasRef = ref<HTMLCanvasElement | null>(null);
const { reducedMotion } = useReducedMotion();
const budget = particleBudget();
const enabled = computed(() => !reducedMotion.value && budget > 0);

let ctx: CanvasRenderingContext2D | null = null;
let particles: FieldParticle[] = [];
let meteor: Meteor | null = null;
let nextMeteorAt = 0;
let rafId = 0;
let width = 0;
let height = 0;
let mouseX = -9999;
let mouseY = -9999;
let resizeObserver: ResizeObserver | null = null;
let hostEl: HTMLElement | null = null;

const REPEL_RADIUS = 120;
const LINK_RADIUS = 90;

function seed() {
  particles = seedField(width, height, budget);
}

function step(ts: number) {
  if (!ctx) return;
  ctx.clearRect(0, 0, width, height);

  // 视差双层：鼠标在画布内才偏移（离开归零）；按深度预计算两层偏移，避免逐粒子分配
  const mouseInside = mouseX > -999;
  const offFar = mouseInside ? parallaxOffset(mouseX, mouseY, width, height, 0.35) : { x: 0, y: 0 };
  const offNear = mouseInside ? parallaxOffset(mouseX, mouseY, width, height, 1) : { x: 0, y: 0 };

  for (const p of particles) {
    // 鼠标斥力
    const dx = p.x - mouseX;
    const dy = p.y - mouseY;
    const dist = Math.hypot(dx, dy);
    if (dist < REPEL_RADIUS && dist > 0.1) {
      const force = ((REPEL_RADIUS - dist) / REPEL_RADIUS) * 0.6;
      p.vx += (dx / dist) * force;
      p.vy += (dy / dist) * force;
    }
    // 阻尼 + 漂移
    p.vx *= 0.985;
    p.vy *= 0.985;
    p.x += p.vx;
    p.y += p.vy;
    // 边界回绕
    if (p.x < -10) p.x = width + 10;
    else if (p.x > width + 10) p.x = -10;
    if (p.y < -10) p.y = height + 10;
    else if (p.y > height + 10) p.y = -10;

    // 视差后的绘制坐标（连线共用）
    const off = p.depth < 1 ? offFar : offNear;
    p.px = p.x + off.x;
    p.py = p.y + off.y;

    // 星光闪烁：透明度正弦振荡
    ctx.beginPath();
    ctx.arc(p.px, p.py, p.r, 0, Math.PI * 2);
    ctx.fillStyle = `hsla(${p.hue}, 95%, 72%, ${twinkleAlpha(p.phase, ts).toFixed(3)})`;
    ctx.fill();
  }

  // 近距连线（绘制坐标系）
  for (let i = 0; i < particles.length; i++) {
    for (let j = i + 1; j < particles.length; j++) {
      const a = particles[i];
      const b = particles[j];
      const d = Math.hypot(a.px - b.px, a.py - b.py);
      if (d < LINK_RADIUS) {
        ctx.beginPath();
        ctx.moveTo(a.px, a.py);
        ctx.lineTo(b.px, b.py);
        ctx.strokeStyle = `rgba(79, 216, 255, ${(1 - d / LINK_RADIUS) * 0.14})`;
        ctx.lineWidth = 0.6;
        ctx.stroke();
      }
    }
  }

  // 流星：到点生成 → 300ms 划出 → 回收并排程下一颗（屏上同时至多 1 颗）
  if (!meteor && ts >= nextMeteorAt) meteor = createMeteor(width, ts);
  if (meteor) {
    const prog = meteorProgress(meteor, ts);
    if (prog >= 1) {
      meteor = null;
      nextMeteorAt = ts + nextMeteorDelay();
    } else {
      const head = meteorHead(meteor, ts);
      const tailX = head.x - meteor.dx * meteor.length;
      const tailY = head.y - meteor.dy * meteor.length;
      const fade = Math.sin(prog * Math.PI); // 淡入淡出
      const grad = ctx.createLinearGradient(tailX, tailY, head.x, head.y);
      grad.addColorStop(0, 'rgba(140, 220, 255, 0)');
      grad.addColorStop(1, `rgba(220, 245, 255, ${(fade * 0.9).toFixed(3)})`);
      ctx.beginPath();
      ctx.moveTo(tailX, tailY);
      ctx.lineTo(head.x, head.y);
      ctx.strokeStyle = grad;
      ctx.lineWidth = 1.4;
      ctx.stroke();
    }
  }

  rafId = requestAnimationFrame(step);
}

function start() {
  cancelAnimationFrame(rafId);
  if (!document.hidden) rafId = requestAnimationFrame(step);
}

function resize() {
  const canvas = canvasRef.value;
  if (!canvas || !canvas.parentElement) return;
  const rect = canvas.parentElement.getBoundingClientRect();
  const dpr = Math.min(window.devicePixelRatio || 1, 2);
  width = rect.width;
  height = rect.height;
  canvas.width = width * dpr;
  canvas.height = height * dpr;
  ctx = canvas.getContext('2d');
  ctx?.setTransform(dpr, 0, 0, dpr, 0, 0);
  seed();
}

function onMouseMove(e: MouseEvent) {
  const canvas = canvasRef.value;
  if (!canvas) return;
  const rect = canvas.getBoundingClientRect();
  mouseX = e.clientX - rect.left;
  mouseY = e.clientY - rect.top;
}

function onVisibility() {
  if (document.hidden) cancelAnimationFrame(rafId);
  else start();
}

function onMouseLeave() {
  mouseX = -9999;
  mouseY = -9999;
}

onMounted(() => {
  if (!enabled.value) return;
  const canvas = canvasRef.value;
  if (!canvas || !canvas.parentElement) return;
  hostEl = canvas.parentElement;
  resize();
  nextMeteorAt = performance.now() + nextMeteorDelay();
  resizeObserver = new ResizeObserver(resize);
  resizeObserver.observe(hostEl);
  hostEl.addEventListener('mousemove', onMouseMove);
  hostEl.addEventListener('mouseleave', onMouseLeave);
  document.addEventListener('visibilitychange', onVisibility);
  start();
});

onBeforeUnmount(() => {
  cancelAnimationFrame(rafId);
  resizeObserver?.disconnect();
  hostEl?.removeEventListener('mousemove', onMouseMove);
  hostEl?.removeEventListener('mouseleave', onMouseLeave);
  document.removeEventListener('visibilitychange', onVisibility);
});

// reduced-motion 运行期被打开时停止渲染循环
watch(enabled, (on) => {
  if (!on) cancelAnimationFrame(rafId);
});
</script>

<style lang="sass" scoped>
.kb-particle-field
  position: absolute
  inset: 0
  width: 100%
  height: 100%
  pointer-events: none
  z-index: 0
</style>
