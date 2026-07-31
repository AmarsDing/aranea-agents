<template>
  <div class="category-tree">
    <button
      type="button"
      :class="['category-tree__all', { 'category-tree__item--active': selected === '' }]"
      @click="emit('select', '')"
    >
      <q-icon name="apps" size="16px" />
      <span>{{ t('shopPage.allCategories') }}</span>
    </button>

    <div v-for="l1 in nodes" :key="l1.key" class="category-tree__group">
      <button type="button" class="category-tree__l1" @click="toggle(l1.key)">
        <q-icon :name="l1.icon ?? 'folder_open'" size="16px" class="category-tree__l1-icon" />
        <span class="col ellipsis">{{ l1.label }}</span>
        <q-icon :name="isOpen(l1.key) ? 'expand_less' : 'expand_more'" size="16px" />
      </button>

      <div v-show="isOpen(l1.key)" class="category-tree__children">
        <template v-for="l2 in l1.children ?? []" :key="l2.key">
          <button
            v-if="!l2.children?.length"
            type="button"
            :class="['category-tree__l2', { 'category-tree__item--active': selected === l2.key }]"
            @click="emit('select', l2.key)"
          >
            {{ l2.label }}
          </button>
          <div v-else>
            <button
              type="button"
              :class="['category-tree__l2', { 'category-tree__item--active': selected === l2.key }]"
              @click="emit('select', l2.key)"
            >
              <span class="col ellipsis text-left">{{ l2.label }}</span>
              <q-icon :name="isOpen(l2.key) ? 'expand_less' : 'expand_more'" size="14px" @click.stop="toggle(l2.key)" />
            </button>
            <div v-show="isOpen(l2.key)" class="category-tree__leaves">
              <button
                v-for="l3 in l2.children"
                :key="l3.key"
                type="button"
                :class="['category-tree__l3', { 'category-tree__item--active': selected === l3.key }]"
                @click="emit('select', l3.key)"
              >
                {{ l3.label }}
              </button>
            </div>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { CategoryNode } from '../../features/ecosystem/types';

const props = defineProps<{
  nodes: CategoryNode[];
  selected: string;
}>();

const emit = defineEmits<{
  select: [key: string];
}>();

const { t } = useI18n();

const openKeys = ref<Set<string>>(new Set());

function isOpen(key: string): boolean {
  return openKeys.value.has(key);
}

function toggle(key: string) {
  const next = new Set(openKeys.value);
  if (next.has(key)) next.delete(key);
  else next.add(key);
  openKeys.value = next;
}

/** 选中某个分类时，自动展开其祖先节点 */
watch(
  () => props.selected,
  (key) => {
    if (!key) return;
    const parts = key.split('/');
    const next = new Set(openKeys.value);
    for (let i = 1; i < parts.length; i++) next.add(parts.slice(0, i).join('/'));
    openKeys.value = next;
  },
  { immediate: true },
);
</script>

<style scoped>
.category-tree {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 13px;
}
.category-tree button {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  border: none;
  background: transparent;
  cursor: pointer;
  border-radius: 8px;
  color: var(--color-text-secondary);
  font-size: 13px;
  line-height: 1.4;
  text-align: left;
  transition:
    background 0.15s ease,
    color 0.15s ease;
}
.category-tree button:hover {
  background: var(--interaction-surface-hover);
  color: var(--color-text-primary);
}
body.body--dark .category-tree button:hover {
  background: rgba(255, 255, 255, 0.06);
}
.category-tree__all {
  padding: 8px 10px;
  font-weight: 600;
}
.category-tree__l1 {
  padding: 8px 10px;
  font-weight: 600;
  color: var(--color-text-primary);
}
.category-tree__l1-icon {
  color: var(--color-icon-muted);
}
.category-tree__children {
  padding-left: 10px;
}
.category-tree__l2 {
  padding: 6px 10px;
  font-weight: 500;
}
.category-tree__leaves {
  padding-left: 14px;
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.category-tree__l3 {
  padding: 5px 10px;
  font-size: 12px;
}
.category-tree__item--active,
.category-tree__item--active:hover {
  background: var(--color-accent);
  color: #fff !important;
  font-weight: 600;
}
</style>
