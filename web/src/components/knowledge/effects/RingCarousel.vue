<template>
  <div class="kb-ring" :class="{ 'kb-ring--flat': reducedMotion }">
    <!-- reduced-motion 降级：纵向列表 -->
    <div v-if="reducedMotion" class="kb-ring__flat-list">
      <button
        v-for="item in items"
        :key="item.key"
        type="button"
        class="kb-ring__flat-item kb-glass"
        @click="$emit('select', item)"
      >
        <q-icon :name="item.icon || 'description'" size="18px" class="kb-ring__flat-icon" />
        <span class="kb-ring__flat-title">{{ item.title }}</span>
        <span v-if="item.subtitle" class="kb-ring__flat-subtitle kb-text-dim">{{ item.subtitle }}</span>
      </button>
    </div>

    <!-- 3D 环形 -->
    <div v-else class="kb-ring__stage" @mouseenter="paused = true" @mouseleave="paused = false">
      <div class="kb-ring__ring" :style="ringStyle">
        <button
          v-for="(item, i) in items"
          :key="item.key"
          type="button"
          class="kb-ring__card kb-glass"
          :style="cardStyle(i)"
          @click="$emit('select', item)"
        >
          <q-icon :name="item.icon || 'description'" size="22px" class="kb-ring__card-icon" />
          <span class="kb-ring__card-title">{{ item.title }}</span>
          <span v-if="item.subtitle" class="kb-ring__card-subtitle kb-text-dim">{{ item.subtitle }}</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// 空态 3D 环形 Carousel（SP2 §SP2-3）：卡片环形排布自动旋转，hover 暂停，点击进入。
// 降级：reduced-motion 退化为纵向列表。
import { computed, ref } from 'vue';
import { useReducedMotion } from '../../../features/knowledge/useReducedMotion';

export type RingItem = { key: string; title: string; subtitle?: string; icon?: string };

const props = defineProps<{ items: RingItem[] }>();
defineEmits<{ (e: 'select', item: RingItem): void }>();

const { reducedMotion } = useReducedMotion();
const paused = ref(false);

const RADIUS = 220;

const step = computed(() => (props.items.length ? 360 / props.items.length : 0));
const ringStyle = computed(() => ({
  animationPlayState: paused.value ? 'paused' : 'running',
}));

function cardStyle(i: number) {
  return {
    transform: `rotateY(${(i * step.value).toFixed(2)}deg) translateZ(${RADIUS}px)`,
  };
}
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
    height: 320px
    display: flex
    align-items: center
    justify-content: center

  &__ring
    position: relative
    width: 200px
    height: 240px
    transform-style: preserve-3d
    animation: kb-ring-spin 24s linear infinite

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
    transition: border-color 160ms ease-out, box-shadow 160ms ease-out
    backface-visibility: hidden

    &:hover
      border-color: var(--kb-accent-cyan)
      box-shadow: 0 0 24px rgba(79, 216, 255, 0.28)

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

@keyframes kb-ring-spin
  from
    transform: rotateY(0deg)
  to
    transform: rotateY(360deg)
</style>
