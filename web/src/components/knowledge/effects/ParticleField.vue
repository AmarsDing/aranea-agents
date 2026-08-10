<template>
  <canvas v-if="enabled" ref="canvasRef" class="kb-particle-field" aria-hidden="true" />
</template>

<script setup lang="ts">
// 深空粒子背景（SP2 §SP2-3）：Canvas 2D 漂浮粒子 + 鼠标斥力 + 近距连线。
// 降级：reduced-motion 或低端设备（particleBudget()=0）时不渲染；页面不可见时停帧。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { particleBudget, useReducedMotion } from '../../../features/knowledge/useReducedMotion';

type Particle = { x: number; y: number; vx: number; vy: number; r: number; hue: number };

const canvasRef = ref<HTMLCanvasElement | null>(null);
const { reducedMotion } = useReducedMotion();
const budget = particleBudget();
const enabled = computed(() => !reducedMotion.value && budget > 0);

let ctx: CanvasRenderingContext2D | null = null;
let particles: Particle[] = [];
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
  particles = Array.from({ length: budget }, () => ({
    x: Math.random() * width,
    y: Math.random() * height,
    vx: (Math.random() - 0.5) * 0.22,
    vy: (Math.random() - 0.5) * 0.22,
    r: 0.8 + Math.random() * 1.6,
    hue: Math.random() < 0.72 ? 197 : 262, // cyan / violet
  }));
}

function step() {
  if (!ctx) return;
  ctx.clearRect(0, 0, width, height);

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

    ctx.beginPath();
    ctx.arc(p.x, p.y, p.r, 0, Math.PI * 2);
    ctx.fillStyle = `hsla(${p.hue}, 95%, 72%, 0.55)`;
    ctx.fill();
  }

  // 近距连线
  for (let i = 0; i < particles.length; i++) {
    for (let j = i + 1; j < particles.length; j++) {
      const a = particles[i];
      const b = particles[j];
      const d = Math.hypot(a.x - b.x, a.y - b.y);
      if (d < LINK_RADIUS) {
        ctx!.beginPath();
        ctx!.moveTo(a.x, a.y);
        ctx!.lineTo(b.x, b.y);
        ctx!.strokeStyle = `rgba(79, 216, 255, ${(1 - d / LINK_RADIUS) * 0.14})`;
        ctx!.lineWidth = 0.6;
        ctx!.stroke();
      }
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
