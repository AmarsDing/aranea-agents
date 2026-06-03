<template>
  <taxonomy-picker
    :model-value="modelValue"
    :tree="tree"
    label="业务分类"
    placeholder="全部业务分类"
    selectable-level="any"
    caption-mode="level"
    icon="filter_alt"
    control-class="agent-control"
    @update:model-value="$emit('update:modelValue', $event)"
  />
</template>

<script setup lang="ts">
import type { PlatformResourceTreeNode } from '../../features/platform/types';
import TaxonomyPicker from './TaxonomyPicker.vue';

defineProps<{
  modelValue: string | null;
  tree: PlatformResourceTreeNode[];
}>();

defineEmits<{
  'update:modelValue': [value: string | null];
}>();
</script>
