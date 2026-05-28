<template>
  <Teleport to="body">
    <div
      v-if="visible"
      class="graph-ctx-menu"
      :style="{ left: `${x}px`, top: `${y}px` }"
      @mousedown.stop
      @contextmenu.prevent
    >
      <div class="graph-ctx-menu__top-line" />
      <div
        v-for="item in items"
        :key="item.action"
        class="graph-ctx-menu__item"
        :class="{
          'graph-ctx-menu__item--danger': item.danger,
          'graph-ctx-menu__item--success': item.success,
        }"
        @click="onSelect(item.action)"
        @mousedown="onPress($event)"
      >
        <span class="graph-ctx-menu__icon">{{ item.icon }}</span>
        <span class="graph-ctx-menu__label">{{ item.label }}</span>
        <span v-if="item.shortcut" class="graph-ctx-menu__shortcut">{{ item.shortcut }}</span>
      </div>
      <div class="graph-ctx-menu__bottom-line" />
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted } from "vue";

export type ContextMenuItem = {
  icon: string;
  label: string;
  shortcut?: string;
  danger?: boolean;
  success?: boolean;
  action: string;
};

defineProps<{
  visible: boolean;
  x: number;
  y: number;
  items: ContextMenuItem[];
}>();

const emit = defineEmits<{
  select: [action: string];
  close: [];
}>();

function onSelect(action: string) {
  emit("select", action);
  emit("close");
}

function onPress(e: MouseEvent) {
  const el = e.currentTarget as HTMLElement;
  el.style.transform = "scale(0.97)";
  const onUp = () => {
    el.style.transform = "";
    window.removeEventListener("mouseup", onUp);
  };
  window.addEventListener("mouseup", onUp);
}

function onDocClick(e: MouseEvent) {
  const menu = document.querySelector(".graph-ctx-menu");
  if (menu && !menu.contains(e.target as Node)) {
    emit("close");
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === "Escape") {
    emit("close");
  }
}

onMounted(() => {
  document.addEventListener("mousedown", onDocClick, true);
  document.addEventListener("keydown", onKeydown, true);
});

onUnmounted(() => {
  document.removeEventListener("mousedown", onDocClick, true);
  document.removeEventListener("keydown", onKeydown, true);
});
</script>

<style scoped>
.graph-ctx-menu {
  position: fixed;
  z-index: 9999;
  min-width: 210px;
  background: rgba(3, 7, 18, 0.97);
  backdrop-filter: blur(28px);
  border: 1px solid rgba(34, 211, 238, 0.12);
  border-radius: 10px;
  box-shadow:
    0 0 20px rgba(34, 211, 238, 0.08),
    0 0 60px rgba(34, 211, 238, 0.04),
    0 8px 32px rgba(0, 0, 0, 0.5);
  padding: 4px 0;
  animation: ctx-menu-in 0.12s ease-out;
}

@keyframes ctx-menu-in {
  from {
    opacity: 0;
    transform: scale(0.95) translateY(-4px);
  }
  to {
    opacity: 1;
    transform: scale(1) translateY(0);
  }
}

.graph-ctx-menu__top-line,
.graph-ctx-menu__bottom-line {
  height: 1px;
  background: linear-gradient(90deg, transparent, rgba(34, 211, 238, 0.5), transparent);
  margin: 0 8px;
}

.graph-ctx-menu__top-line {
  margin-bottom: 4px;
}

.graph-ctx-menu__bottom-line {
  margin-top: 4px;
}

.graph-ctx-menu__item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 14px;
  margin: 2px 4px;
  border-radius: 4px;
  cursor: pointer;
  position: relative;
  transition: background 0.1s, transform 0.08s;
}

.graph-ctx-menu__item::before {
  content: "";
  position: absolute;
  left: 0;
  top: 4px;
  bottom: 4px;
  width: 2px;
  border-radius: 1px;
  background: #22d3ee;
  opacity: 0;
  transition: opacity 0.12s;
  box-shadow: 0 0 6px rgba(34, 211, 238, 0.6);
}

.graph-ctx-menu__item:hover {
  background: rgba(34, 211, 238, 0.06);
}

.graph-ctx-menu__item:hover::before {
  opacity: 1;
}

.graph-ctx-menu__item:active {
  background: rgba(34, 211, 238, 0.12);
}

.graph-ctx-menu__item--danger::before {
  background: #f472b6;
  box-shadow: 0 0 6px rgba(244, 114, 182, 0.6);
}

.graph-ctx-menu__item--danger:hover {
  background: rgba(244, 114, 182, 0.06);
}

.graph-ctx-menu__item--danger:active {
  background: rgba(244, 114, 182, 0.12);
}

.graph-ctx-menu__item--success::before {
  background: #34d399;
  box-shadow: 0 0 6px rgba(52, 211, 153, 0.6);
}

.graph-ctx-menu__item--success:hover {
  background: rgba(52, 211, 153, 0.06);
}

.graph-ctx-menu__item--success:active {
  background: rgba(52, 211, 153, 0.12);
}

.graph-ctx-menu__icon {
  width: 20px;
  text-align: center;
  font-size: 16px;
  color: #22d3ee;
  font-weight: 700;
  flex-shrink: 0;
  transition: filter 0.12s;
}

.graph-ctx-menu__item:hover .graph-ctx-menu__icon {
  filter: drop-shadow(0 0 4px rgba(34, 211, 238, 0.5));
}

.graph-ctx-menu__item--danger .graph-ctx-menu__icon {
  color: #f472b6;
}

.graph-ctx-menu__item--danger:hover .graph-ctx-menu__icon {
  filter: drop-shadow(0 0 4px rgba(244, 114, 182, 0.5));
}

.graph-ctx-menu__item--success .graph-ctx-menu__icon {
  color: #34d399;
}

.graph-ctx-menu__item--success:hover .graph-ctx-menu__icon {
  filter: drop-shadow(0 0 4px rgba(52, 211, 153, 0.5));
}

.graph-ctx-menu__label {
  flex: 1;
  color: #a5f3fc;
  font-size: 12px;
  font-weight: 500;
  letter-spacing: 0.03em;
  white-space: nowrap;
}

.graph-ctx-menu__shortcut {
  padding: 1px 5px;
  border-radius: 3px;
  background: rgba(34, 211, 238, 0.06);
  color: #22d3ee;
  font-size: 9px;
  font-weight: 600;
  border: 1px solid rgba(34, 211, 238, 0.12);
  font-family: ui-monospace, monospace;
  flex-shrink: 0;
}
</style>
