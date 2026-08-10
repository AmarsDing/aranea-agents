<template>
  <button
    ref="btnRef"
    type="button"
    class="kb-glow-btn"
    :class="{ 'kb-glow-btn--ghost': ghost, 'kb-glow-btn--sm': sm }"
    :style="magnetStyle"
    :disabled="disabled"
    @mousemove="onMouseMove"
    @mouseleave="onMouseLeave"
    @click="$emit('click', $event)"
  >
    <q-icon v-if="icon" :name="icon" :size="sm ? '14px' : '16px'" />
    <span v-if="label" class="kb-glow-btn__label">{{ label }}</span>
    <slot />
  </button>
</template>

<script setup lang="ts">
// 辉光磁吸按钮（SP2 §SP2-3）：hover 辉光晕 + 鼠标磁吸位移（≤6px）。
// 降级：reduced-motion 时仅保留颜色 hover，无磁吸/辉光动画。
import { computed, ref } from 'vue';
import { useReducedMotion } from '../../../features/knowledge/useReducedMotion';

defineProps<{
  label?: string;
  icon?: string;
  ghost?: boolean;
  sm?: boolean;
  disabled?: boolean;
}>();
defineEmits<{ (e: 'click', ev: MouseEvent): void }>();

const MAX_MAGNET = 6;

const btnRef = ref<HTMLElement | null>(null);
const { reducedMotion } = useReducedMotion();
const mx = ref(0);
const my = ref(0);

const magnetStyle = computed(() => {
  if (reducedMotion.value) return {};
  return { transform: `translate(${mx.value.toFixed(1)}px, ${my.value.toFixed(1)}px)` };
});

function onMouseMove(e: MouseEvent) {
  if (reducedMotion.value) return;
  const el = btnRef.value;
  if (!el) return;
  const rect = el.getBoundingClientRect();
  mx.value = ((e.clientX - rect.left) / rect.width - 0.5) * 2 * MAX_MAGNET;
  my.value = ((e.clientY - rect.top) / rect.height - 0.5) * 2 * MAX_MAGNET;
}

function onMouseLeave() {
  mx.value = 0;
  my.value = 0;
}
</script>

<style lang="sass" scoped>
.kb-glow-btn
  display: inline-flex
  align-items: center
  gap: 6px
  padding: 8px 18px
  border-radius: 10px
  border: 1px solid var(--kb-accent-cyan-dim)
  background: linear-gradient(135deg, rgba(79, 216, 255, 0.16), rgba(157, 107, 255, 0.12))
  color: var(--kb-accent-cyan)
  font-size: 13px
  font-weight: 600
  letter-spacing: 0.02em
  cursor: pointer
  transition: transform 120ms ease-out, box-shadow 160ms ease-out, border-color 160ms ease-out
  will-change: transform

  &:hover:not(:disabled)
    border-color: var(--kb-accent-cyan)
    box-shadow: 0 0 18px rgba(79, 216, 255, 0.35), 0 0 42px rgba(79, 216, 255, 0.12)

  &:active:not(:disabled)
    box-shadow: 0 0 8px rgba(79, 216, 255, 0.5)

  &:disabled
    opacity: 0.45
    cursor: not-allowed

  &--ghost
    background: transparent
    border-color: var(--kb-glass-border)
    color: var(--kb-text-dim)

    &:hover:not(:disabled)
      color: var(--kb-accent-cyan)
      border-color: var(--kb-accent-cyan-dim)
      box-shadow: 0 0 12px rgba(79, 216, 255, 0.18)

  &--sm
    padding: 4px 12px
    font-size: 12px
    border-radius: 8px
</style>
