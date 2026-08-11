<template>
  <div
    ref="panelEl"
    class="kb-glass kb-glass-panel"
    :class="{
      'kb-glass--strong': strong,
      'kb-glass-panel--glow': glow,
      'kb-glass-panel--refract': refract,
    }"
    @pointermove="onPointerMove"
  >
    <!-- 液态玻璃装饰层：SVG 折射光纹 + 指针追随高光，均不拦截交互。
         滤镜定义由 LiquidGlassDefs 单例提供（Workbench 根挂载）。 -->
    <div class="kb-glass-panel__sheen" aria-hidden="true" />
    <div class="kb-glass-panel__highlight" aria-hidden="true" />
    <div v-if="title" class="kb-glass-panel__header">
      <q-icon v-if="icon" :name="icon" size="16px" class="kb-glass-panel__icon" />
      <span class="kb-glass-panel__title">{{ title }}</span>
      <slot name="header-actions" />
    </div>
    <div class="kb-glass-panel__body" :class="{ 'kb-glass-panel__body--flush': flush }">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
// 液态玻璃容器（SP2 §SP2-3 + P0-2 液态化）：面板/浮层基座。
// 三层液态效果：
//   1. SVG 位移滤镜驱动的折射光纹（__sheen，filter: url() 作用于装饰层，Chrome 安全）
//   2. 镜面边缘（::after 内阴影：顶缘高光 / 底缘暗边，模拟玻璃断面反光）
//   3. 指针追随高光（__highlight，--kb-mx/--kb-my 由 pointermove 驱动）
// 装饰层统一带边缘羽化 mask，避免硬边。视觉令牌来自 css/deep-space.sass（.kb-workbench 作用域）。
import { ref } from 'vue';

defineProps<{
  title?: string;
  icon?: string;
  /** 更强的玻璃底色（浮层用） */
  strong?: boolean;
  /** 呼吸辉光 */
  glow?: boolean;
  /** 去掉 body 内边距（编辑器/列表等自管理padding的内容） */
  flush?: boolean;
  /** M1：背景真折射（backdrop-filter: url(#kb-liquid-bg)；@supports 降级普通 blur） */
  refract?: boolean;
}>();

const panelEl = ref<HTMLElement>();

function onPointerMove(e: PointerEvent): void {
  const el = panelEl.value;
  if (!el) return;
  const rect = el.getBoundingClientRect();
  if (rect.width === 0 || rect.height === 0) return;
  el.style.setProperty('--kb-mx', `${(((e.clientX - rect.left) / rect.width) * 100).toFixed(2)}%`);
  el.style.setProperty('--kb-my', `${(((e.clientY - rect.top) / rect.height) * 100).toFixed(2)}%`);
}
</script>

<style lang="sass" scoped>
.kb-glass-panel
  display: flex
  flex-direction: column
  min-height: 0
  isolation: isolate

  // ── 层 1：折射光纹（SVG 位移滤镜 + 对角光泽，静态无动画）──
  &__sheen
    position: absolute
    inset: 0
    border-radius: inherit
    pointer-events: none
    z-index: 0
    background: linear-gradient(115deg, rgba(255, 255, 255, 0.07) 0%, transparent 30%, transparent 68%, rgba(255, 255, 255, 0.05) 100%)
    filter: url(#kb-liquid-refract)
    // 边缘羽化：光纹在角落淡出，避免硬边
    mask-image: radial-gradient(130% 130% at 50% 50%, black 62%, transparent 100%)
    -webkit-mask-image: radial-gradient(130% 130% at 50% 50%, black 62%, transparent 100%)

  // ── 层 3：指针追随高光（悬停显现）──
  &__highlight
    position: absolute
    inset: 0
    border-radius: inherit
    pointer-events: none
    z-index: 0
    opacity: 0
    transition: opacity 0.35s ease
    background: radial-gradient(240px circle at var(--kb-mx, 50%) var(--kb-my, 50%), rgba(255, 255, 255, 0.1), transparent 70%)
    mask-image: radial-gradient(130% 130% at 50% 50%, black 62%, transparent 100%)
    -webkit-mask-image: radial-gradient(130% 130% at 50% 50%, black 62%, transparent 100%)

  &:hover > &__highlight
    opacity: 1

  // ── 层 2：镜面边缘（顶/左缘反光，底缘暗线 → 玻璃断面感）──
  &::after
    content: ''
    position: absolute
    inset: 0
    border-radius: inherit
    pointer-events: none
    z-index: 2
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.12), inset 1px 0 0 rgba(255, 255, 255, 0.05), inset 0 -1px 0 rgba(0, 0, 0, 0.3)

  &--glow
    animation: kb-glass-breathe 4.5s ease-in-out infinite

  &__header
    position: relative
    z-index: 1
    display: flex
    align-items: center
    gap: 8px
    padding: 10px 14px
    border-bottom: 1px solid var(--kb-glass-border)
    flex: none

  &__icon
    color: var(--kb-accent-cyan)

  &__title
    font-size: 12px
    font-weight: 600
    letter-spacing: 0.08em
    text-transform: uppercase
    color: var(--kb-text-dim)
    flex: 1

  &__body
    position: relative
    z-index: 1
    padding: 12px 14px
    flex: 1
    min-height: 0
    overflow: auto

    &--flush
      padding: 0

@keyframes kb-glass-breathe
  0%, 100%
    box-shadow: inset 0 0 24px color-mix(in srgb, var(--color-accent) 4%, transparent), 0 12px 40px rgba(2, 6, 18, 0.5)
  50%
    box-shadow: inset 0 0 32px color-mix(in srgb, var(--color-accent) 10%, transparent), 0 12px 48px color-mix(in srgb, var(--color-accent) 12%, transparent)
</style>
