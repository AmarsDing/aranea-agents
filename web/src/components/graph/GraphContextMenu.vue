<template>
  <Teleport to="body">
    <div
      v-if="visible"
      class="graph-ctx-menu"
      role="menu"
      aria-label="节点操作菜单"
      :style="{ left: `${x}px`, top: `${y}px` }"
      @mousedown.stop
      @contextmenu.prevent
    >
      <div class="graph-ctx-menu__top-line" />
      <div
        v-for="item in items"
        :key="item.action"
        class="graph-ctx-menu__item"
        role="menuitem"
        tabindex="0"
        :class="{
          'graph-ctx-menu__item--danger': item.danger,
          'graph-ctx-menu__item--success': item.success,
        }"
        @click="onSelect(item.action)"
        @keydown.enter="onSelect(item.action)"
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
