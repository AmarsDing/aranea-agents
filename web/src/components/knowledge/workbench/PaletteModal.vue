<template>
  <!-- 液态玻璃模态框架（SP2 §SP2-7）：遮罩 blur + 居中 GlassPanel；⌘O/⌘K 两浮层共用 -->
  <div class="kb-palette-overlay" @mousedown.self="$emit('close')">
    <GlassPanel strong class="kb-palette" role="dialog" :aria-label="title">
      <div class="kb-palette__input-row">
        <q-icon :name="icon" size="18px" class="kb-palette__input-icon" />
        <input
          ref="inputRef"
          :value="query"
          :placeholder="placeholder"
          class="kb-palette__input"
          @input="$emit('update:query', ($event.target as HTMLInputElement).value)"
          @keydown="$emit('keydown', $event)"
        />
        <slot name="input-actions" />
      </div>
      <div class="kb-palette__body">
        <slot />
      </div>
    </GlassPanel>
  </div>
</template>

<script setup lang="ts">
// PaletteModal：⌘O 快速切换 / ⌘K 命令面板共用的居中模态结构（SP2 §SP2-7）。
import { nextTick, ref, watch } from 'vue';
import GlassPanel from '../effects/GlassPanel.vue';

const props = defineProps<{
  open: boolean;
  title: string;
  icon: string;
  placeholder: string;
  query: string;
}>();

defineEmits<{
  close: [];
  'update:query': [v: string];
  keydown: [e: KeyboardEvent];
}>();

const inputRef = ref<HTMLInputElement | null>(null);

watch(
  () => props.open,
  async (on) => {
    if (on) {
      await nextTick();
      inputRef.value?.focus();
    }
  },
  { immediate: true },
);

function focus() {
  inputRef.value?.focus();
}

defineExpose({ focus });
</script>

<style lang="sass" scoped>
.kb-palette-overlay
  position: absolute
  inset: 0
  z-index: 30
  display: flex
  align-items: flex-start
  justify-content: center
  padding-top: 12vh
  backdrop-filter: blur(6px) brightness(0.6)
  -webkit-backdrop-filter: blur(6px) brightness(0.6)
  background: rgba(4, 8, 18, 0.45)

.kb-palette
  width: 560px
  max-width: 92vw
  max-height: 60vh

  &__input-row
    display: flex
    align-items: center
    gap: 10px
    padding: 12px 16px
    border-bottom: 1px solid var(--kb-glass-border)

  &__input-icon
    color: var(--kb-accent-cyan)
    flex: none

  &__input
    flex: 1
    min-width: 0
    border: 0
    outline: 0
    background: transparent
    color: var(--kb-text-primary)
    font-size: 14.5px

    &::placeholder
      color: var(--kb-text-dim)

  &__body
    overflow-y: auto
    max-height: calc(60vh - 52px)
    padding: 6px
</style>
