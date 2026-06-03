<template>
  <Teleport to="body">
    <div
      v-if="visible"
      ref="menuRef"
      :class="['graph-ctx-menu', { 'graph-ctx-menu--danger': hasDangerItem }]"
      :style="{ left: `${clampedX}px`, top: `${clampedY}px` }"
      @contextmenu.prevent
      @mousedown.stop
    >
      <div class="graph-ctx-menu__glow-top" />
      <div
        v-for="(item, idx) in items"
        :key="idx"
        class="graph-ctx-menu__item"
        :class="{
          'graph-ctx-menu__item--danger': item.danger,
          'graph-ctx-menu__item--success': item.success,
          'graph-ctx-menu__item--disabled': !!item.disabled,
        }"
        @click.stop="onItemClick(item)"
      >
        <div class="graph-ctx-menu__accent" />
        <span class="graph-ctx-menu__icon">{{ item.icon }}</span>
        <span class="graph-ctx-menu__label">{{ item.label }}</span>
        <span v-if="item.shortcut" class="graph-ctx-menu__shortcut">{{ item.shortcut }}</span>
      </div>
      <div class="graph-ctx-menu__glow-bottom" />
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue';

export interface ContextMenuItem {
  icon: string;
  label: string;
  shortcut?: string;
  action: string;
  danger?: boolean;
  success?: boolean;
  disabled?: boolean;
}

const props = defineProps<{
  visible: boolean;
  x: number;
  y: number;
  items: ContextMenuItem[];
}>();

const emit = defineEmits<{
  select: [action: string];
  close: [];
}>();

const menuRef = ref<HTMLElement | null>(null);

const MENU_WIDTH = 234;
const ITEM_HEIGHT = 38;
const PADDING = 8;

const clampedX = computed(() => {
  const raw = props.x + PADDING;
  return raw + MENU_WIDTH > window.innerWidth ? window.innerWidth - MENU_WIDTH - PADDING : raw;
});

const clampedY = computed(() => {
  const totalHeight = props.items.length * ITEM_HEIGHT + PADDING * 2;
  const raw = props.y + PADDING;
  return raw + totalHeight > window.innerHeight ? window.innerHeight - totalHeight - PADDING : raw;
});

const hasDangerItem = computed(() => props.items.some((i) => i.danger));

function onItemClick(item: ContextMenuItem) {
  if (item.disabled) return;
  emit('select', item.action);
}

function onDocClick(e: MouseEvent) {
  if (!props.visible) return;
  if (menuRef.value && !menuRef.value.contains(e.target as Node)) {
    emit('close');
  }
}

function onDocKeydown(e: KeyboardEvent) {
  if (!props.visible) return;
  if (e.key === 'Escape') {
    e.preventDefault();
    emit('close');
  }
}

onMounted(() => {
  document.addEventListener('mousedown', onDocClick, true);
  document.addEventListener('keydown', onDocKeydown, true);
});

onUnmounted(() => {
  document.removeEventListener('mousedown', onDocClick, true);
  document.removeEventListener('keydown', onDocKeydown, true);
});

watch(
  () => props.visible,
  async (val) => {
    if (val) {
      await nextTick();
      menuRef.value?.focus({ preventScroll: true });
    }
  },
);
</script>
