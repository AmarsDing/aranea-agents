<template>
  <nav class="skills-tab-nav">
    <button
      v-for="item in items"
      :key="item.id"
      type="button"
      class="skills-tab-nav__item"
      :class="{ 'is-active': item.id === activeId }"
      @click="$emit('select', item.id)"
    >
      {{ item.label }}
    </button>
  </nav>
</template>

<script setup lang="ts">
// 展示组件：Skill/工具 Tab 页内锚点导航（黏性 pill 条，与外层 q-tabs 层级区分）
defineProps<{
  items: { id: string; label: string }[];
  activeId: string;
}>();

defineEmits<{
  select: [id: string];
}>();
</script>
