<template>
  <div v-if="visible" class="graph-node-search">
    <q-input
      ref="inputRef"
      v-model="query"
      dense
      outlined
      clearable
      placeholder="搜索节点…"
      class="graph-node-search__input app-glass-control app-glass-control--sm"
      @update:model-value="$emit('search', query)"
      @keydown.escape.prevent="$emit('close')"
    >
      <template #prepend><q-icon name="search" size="16px" /></template>
      <template #append>
        <span class="graph-node-search__count" v-if="matchCount > 0">{{ matchIndex + 1 }}/{{ matchCount }}</span>
        <q-btn flat dense round icon="keyboard_arrow_up" size="xs" :disable="matchCount <= 1" @click="$emit('prev')" />
        <q-btn flat dense round icon="keyboard_arrow_down" size="xs" :disable="matchCount <= 1" @click="$emit('next')" />
        <q-btn flat dense round icon="close" size="xs" @click="$emit('close')" />
      </template>
    </q-input>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick } from "vue";

const props = defineProps<{
  visible: boolean;
  matchIndex: number;
  matchCount: number;
}>();

defineEmits<{
  search: [query: string];
  prev: [];
  next: [];
  close: [];
}>();

const query = ref("");
const inputRef = ref<any>(null);

watch(() => props.visible, (v) => {
  if (v) {
    query.value = "";
    nextTick(() => inputRef.value?.focus());
  }
});
</script>
