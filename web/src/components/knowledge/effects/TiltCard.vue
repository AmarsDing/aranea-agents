<template>
  <div
    ref="cardRef"
    class="kb-tilt-card"
    :class="{ 'kb-tilt-card--spring': !hovering }"
    :style="cardStyle"
    @mousemove="onMouseMove"
    @mouseleave="onMouseLeave"
  >
    <div class="kb-tilt-card__glare" :style="glareStyle" />
    <div class="kb-tilt-card__content">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
// 3D 倾斜卡片（SP2 §SP2-3）：hover 跟随鼠标 rotateX/Y（±8°）+ 移动高光。
// 降级：reduced-motion 时退化为静态卡（无 transform/高光）。
import { computed, ref } from 'vue';
import { useReducedMotion } from '../../../features/knowledge/useReducedMotion';

const MAX_TILT = 8;

const cardRef = ref<HTMLElement | null>(null);
const { reducedMotion } = useReducedMotion();
const rx = ref(0);
const ry = ref(0);
const glareX = ref(50);
const glareY = ref(50);
const hovering = ref(false);

const cardStyle = computed(() => {
  if (reducedMotion.value || !hovering.value) {
    return { transform: 'perspective(900px) rotateX(0deg) rotateY(0deg)' };
  }
  return {
    transform: `perspective(900px) rotateX(${rx.value.toFixed(2)}deg) rotateY(${ry.value.toFixed(2)}deg)`,
  };
});

const glareStyle = computed(() => {
  if (reducedMotion.value || !hovering.value) return { opacity: 0 };
  return {
    opacity: 1,
    background: `radial-gradient(circle at ${glareX.value}% ${glareY.value}%, rgba(255,255,255,0.12) 0%, transparent 55%)`,
  };
});

function onMouseMove(e: MouseEvent) {
  if (reducedMotion.value) return;
  const el = cardRef.value;
  if (!el) return;
  const rect = el.getBoundingClientRect();
  if (!rect.width || !rect.height) return; // 未布局（jsdom/隐藏态）不倾斜
  const px = (e.clientX - rect.left) / rect.width; // 0..1
  const py = (e.clientY - rect.top) / rect.height;
  hovering.value = true;
  ry.value = (px - 0.5) * 2 * MAX_TILT;
  rx.value = (0.5 - py) * 2 * MAX_TILT;
  glareX.value = px * 100;
  glareY.value = py * 100;
}

function onMouseLeave() {
  hovering.value = false;
}
</script>

<style lang="sass" scoped>
.kb-tilt-card
  position: relative
  border-radius: var(--kb-radius-glass)
  // hover 跟随：快速 ease-out 贴手
  transition: transform 120ms ease-out
  transform-style: preserve-3d
  will-change: transform

  // V3 弹簧回正：离开后以 overshoot 缓动归位（回弹感，方案 §三-V3）
  &--spring
    transition: transform 420ms cubic-bezier(0.34, 1.56, 0.64, 1)

  &__glare
    position: absolute
    inset: 0
    border-radius: inherit
    pointer-events: none
    transition: opacity 180ms ease-out
    z-index: 1

  &__content
    position: relative
    z-index: 2
    height: 100%
</style>
