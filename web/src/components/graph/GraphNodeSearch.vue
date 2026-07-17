<template>
  <div v-if="visible" class="graph-node-search">
    <q-input
      ref="inputRef"
      v-model="query"
      dense
      outlined
      clearable
      :placeholder="t('graphs.nodeSearchPlaceholder')"
      class="graph-node-search__input app-glass-control app-glass-control--sm"
      @update:model-value="$emit('search', query)"
      @keydown.escape.prevent="$emit('close')"
    >
      <template #prepend><q-icon name="search" size="16px" /></template>
      <template #append>
        <span v-if="matchCount > 0" class="graph-node-search__count">{{ matchIndex + 1 }}/{{ matchCount }}</span>
        <q-btn flat dense round icon="keyboard_arrow_up" size="xs" :disable="matchCount <= 1" @click="$emit('prev')" />
        <q-btn
          flat
          dense
          round
          icon="keyboard_arrow_down"
          size="xs"
          :disable="matchCount <= 1"
          @click="$emit('next')"
        />
        <q-btn flat dense round icon="close" size="xs" @click="$emit('close')" />
      </template>
    </q-input>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick } from 'vue';
import { useI18n } from 'vue-i18n';
import type { QInput } from 'quasar';

const { t } = useI18n();

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

const query = ref('');
const inputRef = ref<QInput | null>(null);

watch(
  () => props.visible,
  (v) => {
    if (v) {
      query.value = '';
      nextTick(() => inputRef.value?.focus());
    }
  },
);

void t;
</script>
