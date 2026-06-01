<template>
  <GraphContextMenu
    :visible="visible"
    :x="x"
    :y="y"
    :items="items"
    @select="emit('select', $event)"
    @close="emit('close')"
  />
</template>

<script setup lang="ts">
import GraphContextMenu from "./GraphContextMenu.vue";
import type { ContextMenuItem } from "./GraphContextMenu.vue";

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
</script>
