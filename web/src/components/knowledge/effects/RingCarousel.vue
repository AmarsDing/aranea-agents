<template>
  <div class="kb-ring" :class="{ 'kb-ring--flat': reducedMotion }">
    <!-- reduced-motion 降级：纵向列表 -->
    <div v-if="reducedMotion" class="kb-ring__flat-list">
      <button
        v-for="item in items"
        :key="item.key"
        type="button"
        class="kb-ring__flat-item kb-glass"
        @click="emit('select', item)"
      >
        <q-icon :name="item.icon || 'description'" size="18px" class="kb-ring__flat-icon" />
        <span class="kb-ring__flat-title">{{ item.title }}</span>
        <span v-if="item.subtitle" class="kb-ring__flat-subtitle kb-text-dim">{{ item.subtitle }}</span>
      </button>
    </div>

    <!-- V1 3D 聚焦环：JS 驱动旋转 + 聚焦高亮 + 拖拽/滚轮/惯性吸附 -->
    <div
      v-else
      ref="stageRef"
      class="kb-ring__stage"
      :style="{ height: `${cardHeight + 72}px` }"
      @mouseenter="paused = true"
      @mouseleave="paused = false"
      @pointerdown="onPointerDown"
      @pointermove="onPointerMove"
      @pointerup="onPointerUp"
      @pointercancel="onPointerUp"
      @wheel="onWheel"
    >
      <div ref="ringRef" class="kb-ring__ring" :style="ringSizeStyle">
        <button
          v-for="(item, i) in items"
          :key="item.key"
          type="button"
          class="kb-ring__card kb-glass"
          :style="cardStyle(i)"
          @click="onCardClick(item)"
        >
          <span class="kb-ring__card-halo" aria-hidden="true" />
          <q-icon :name="item.icon || 'description'" size="22px" class="kb-ring__card-icon" />
          <span class="kb-ring__card-title">{{ item.title }}</span>
          <span v-if="item.subtitle" class="kb-ring__card-subtitle kb-text-dim">{{ item.subtitle }}</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// 空态 3D 聚焦环 Carousel（V1，方案 §三-V1）：JS rAF 驱动旋转角度，逐帧计算各卡与正面角差
// 输出 --focus 变量（opacity/blur/scale 衰减）+ 聚焦卡光环；支持拖拽/惯性/吸附/滚轮/hover 暂停。
// 性能纪律：帧循环内只写 transform/opacity CSS 变量与 class，零布局读写；document.hidden 停帧。
// 降级：reduced-motion 退化为纵向列表。
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useReducedMotion } from '../../../features/knowledge/useReducedMotion';

export type RingItem = { key: string; title: string; subtitle?: string; icon?: string };

const props = withDefaults(
  defineProps<{
    items: RingItem[];
    /** 环半径 px（卡片 translateZ） */
    radius?: number;
    /** 自动旋转角速度（度/秒，默认 15 ≈ 24s/圈） */
    speed?: number;
    /** 是否自动旋转（拖拽/滚轮不受影响） */
    autoplay?: boolean;
    /** 卡片尺寸 px */
    cardWidth?: number;
    cardHeight?: number;
  }>(),
  { radius: 220, speed: 15, autoplay: true, cardWidth: 200, cardHeight: 240 },
);

const emit = defineEmits<{ (e: 'select', item: RingItem): void }>();

const { reducedMotion } = useReducedMotion();
const stageRef = ref<HTMLElement | null>(null);
const ringRef = ref<HTMLElement | null>(null);

// ── 交互常量 ──────────────────────────────────────────────
const DRAG_DEG_PER_PX = 0.35; // 拖拽位移 → 角度
const WHEEL_DEG_PER_PX = 0.12; // 滚轮位移 → 角度
const INERTIA_DAMPING = 2.2; // 惯性指数衰减系数（/s）
const SNAP_SPEED = 10; // 吸附插值速率（/s）
const MIN_VELOCITY = 8; // 低于此角速度视为静止（度/秒）
const CLICK_SUPPRESS_PX = 6; // 拖拽位移超过此值抑制 click
const WHEEL_SNAP_MS = 220; // 滚轮停止后吸附延迟

// ── 运行态（帧循环自有，不走 Vue 响应式——避免 60fps 重渲染）──
let rotation = 0; // 环当前角度（度）
let velocity = 0; // 惯性角速度（度/秒）
let dragging = false;
let moved = 0; // 本次拖拽累计位移（click 抑制）
let lastX = 0;
let lastMoveTs = 0;
let wasInertia = false; // 松手时有惯性 → 衰减到阈值后吸附
let snapTarget: number | null = null;
let wheelSnapTimer = 0;
let paused = false;
let rafId = 0;
let lastFrameTs = 0;

const step = computed(() => (props.items.length ? 360 / props.items.length : 0));

const ringSizeStyle = computed(() => ({
  width: `${props.cardWidth}px`,
  height: `${props.cardHeight}px`,
}));

/** 卡片基线 3D 定位（帧循环随后覆写补 scale；绑定值不变时 Vue 不重patch，覆写持续生效）。 */
function cardStyle(i: number) {
  return {
    transform: `rotateY(${(i * step.value).toFixed(2)}deg) translateZ(${props.radius}px)`,
  };
}

/** 单帧写入：环旋转 + 逐卡聚焦变量（--focus 0~1）+ 聚焦类。 */
function applyFrame() {
  const ring = ringRef.value;
  if (!ring) return;
  ring.style.transform = `rotateY(${rotation.toFixed(2)}deg)`;
  const n = props.items.length;
  if (!n) return;
  const stepDeg = 360 / n;
  const cards = ring.querySelectorAll<HTMLElement>('.kb-ring__card');
  cards.forEach((card, i) => {
    // 与正面（0°）的角差，归一化到 [-180, 180)
    const delta = (((i * stepDeg + rotation) % 360) + 540) % 360 - 180;
    const abs = Math.abs(delta);
    // cos 衰减：正面 1 → 90° 侧位 0；背面被 backface-visibility 隐藏，focus 恒 0
    const focus = abs >= 90 ? 0 : Math.cos((abs * Math.PI) / 180);
    const scale = 0.92 + 0.08 * focus;
    card.style.transform = `rotateY(${(i * stepDeg).toFixed(2)}deg) translateZ(${props.radius}px) scale(${scale.toFixed(3)})`;
    card.style.setProperty('--focus', focus.toFixed(3));
    card.classList.toggle('kb-ring__card--focus', abs < stepDeg / 2);
  });
}

function snapToNearest() {
  if (!props.items.length) return;
  // 使 rotation 对齐步进角整数倍 → 最近卡片归正
  snapTarget = Math.round(rotation / step.value) * step.value;
}

function frame(ts: number) {
  const dt = lastFrameTs ? Math.min((ts - lastFrameTs) / 1000, 0.05) : 0;
  lastFrameTs = ts;
  if (!dragging) {
    if (snapTarget !== null) {
      // 吸附：指数插值逼近目标角
      const diff = snapTarget - rotation;
      rotation += diff * Math.min(1, dt * SNAP_SPEED);
      if (Math.abs(diff) < 0.05) {
        rotation = snapTarget;
        snapTarget = null;
      }
    } else if (Math.abs(velocity) > MIN_VELOCITY) {
      // 惯性：衰减直至阈值
      rotation += velocity * dt;
      velocity *= Math.exp(-INERTIA_DAMPING * dt);
    } else {
      velocity = 0;
      if (wasInertia) {
        wasInertia = false;
        snapToNearest();
      } else if (!paused && props.autoplay) {
        rotation += props.speed * dt;
      }
    }
  }
  applyFrame();
  rafId = requestAnimationFrame(frame);
}

// ── 拖拽（pointer）：位移 → 角速度；松手惯性 → 吸附 ──────────
// move/up 绑定在 stage 上 + pointer capture：捕获后事件重定向到 stage，拖出元素不丢跟踪
function onPointerDown(e: PointerEvent) {
  dragging = true;
  moved = 0;
  lastX = e.clientX;
  lastMoveTs = performance.now();
  velocity = 0;
  wasInertia = false;
  snapTarget = null;
  (e.currentTarget as HTMLElement | null)?.setPointerCapture?.(e.pointerId);
}

function onPointerMove(e: PointerEvent) {
  if (!dragging) return;
  const now = performance.now();
  const dx = e.clientX - lastX;
  lastX = e.clientX;
  moved += Math.abs(dx);
  const dDeg = dx * DRAG_DEG_PER_PX;
  rotation += dDeg;
  const dt = Math.max((now - lastMoveTs) / 1000, 0.001);
  lastMoveTs = now;
  velocity = velocity * 0.7 + (dDeg / dt) * 0.3; // 平滑瞬时速度
  applyFrame();
}

function onPointerUp() {
  if (!dragging) return;
  dragging = false;
  if (Math.abs(velocity) > MIN_VELOCITY * 4) {
    wasInertia = true; // 高速松手：帧循环惯性衰减，到阈值后吸附
  } else {
    velocity = 0;
    snapToNearest(); // 低速/静止松手：直接吸附
  }
}

function onWheel(e: WheelEvent) {
  e.preventDefault();
  snapTarget = null;
  velocity = 0;
  wasInertia = false;
  rotation += (e.deltaY || e.deltaX) * WHEEL_DEG_PER_PX;
  applyFrame();
  window.clearTimeout(wheelSnapTimer);
  wheelSnapTimer = window.setTimeout(snapToNearest, WHEEL_SNAP_MS);
}

function onCardClick(item: RingItem) {
  if (moved > CLICK_SUPPRESS_PX) return; // 拖拽后的 click 不触发进入
  emit('select', item);
}

// ── 生命周期：页面不可见停帧 ─────────────────────────────
function onVisibility() {
  if (document.hidden) {
    cancelAnimationFrame(rafId);
    lastFrameTs = 0;
  } else {
    cancelAnimationFrame(rafId);
    rafId = requestAnimationFrame(frame);
  }
}

onMounted(() => {
  if (reducedMotion.value) return;
  applyFrame(); // 首帧同步写入：聚焦高亮立即可见（不等 rAF）
  rafId = requestAnimationFrame(frame);
  document.addEventListener('visibilitychange', onVisibility);
});

onBeforeUnmount(() => {
  cancelAnimationFrame(rafId);
  window.clearTimeout(wheelSnapTimer);
  document.removeEventListener('visibilitychange', onVisibility);
});
</script>

<style lang="sass" scoped>
.kb-ring
  width: 100%
  height: 100%
  display: flex
  align-items: center
  justify-content: center

  &__stage
    perspective: 1100px
    width: 100%
    display: flex
    align-items: center
    justify-content: center
    cursor: grab
    user-select: none
    touch-action: pan-y // 纵向滚动让位页面，横向拖拽转环

    &:active
      cursor: grabbing

  &__ring
    position: relative
    transform-style: preserve-3d
    will-change: transform

  &__card
    position: absolute
    inset: 0
    display: flex
    flex-direction: column
    align-items: center
    justify-content: center
    gap: 8px
    padding: 16px
    cursor: pointer
    color: var(--kb-text-primary)
    backface-visibility: hidden
    // V1 聚焦衰减：帧循环每帧写 --focus；不在 transform/opacity 上加 transition（帧驱动）
    opacity: calc(0.45 + 0.55 * var(--focus, 1))
    filter: blur(calc((1 - var(--focus, 1)) * 2.5px))
    transition: border-color 160ms ease-out, box-shadow 160ms ease-out

    &:hover,
    &--focus
      border-color: var(--kb-accent-cyan)

    &--focus
      box-shadow: 0 0 28px var(--kb-accent-glow)

  &__card-icon
    color: var(--kb-accent-cyan)

  &__card-title
    font-size: 13px
    font-weight: 600
    text-align: center
    word-break: break-all

  &__card-subtitle
    font-size: 11px

  &__flat-list
    display: flex
    flex-direction: column
    gap: 8px
    width: min(420px, 90%)
    max-height: 100%
    overflow: auto

  &__flat-item
    display: flex
    align-items: center
    gap: 10px
    padding: 10px 14px
    cursor: pointer
    color: var(--kb-text-primary)
    text-align: left

    &:hover
      border-color: var(--kb-accent-cyan-dim)

  &__flat-icon
    color: var(--kb-accent-cyan)
    flex: none

  &__flat-title
    font-size: 13px
    font-weight: 600
    flex: 1
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__flat-subtitle
    font-size: 11px
    flex: none
</style>
