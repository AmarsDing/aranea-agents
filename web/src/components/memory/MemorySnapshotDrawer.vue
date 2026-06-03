<template>
  <q-drawer
    :model-value="modelValue"
    side="right"
    overlay
    bordered
    :width="520"
    class="memory-drawer"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <q-scroll-area class="fit">
      <div class="q-pa-md">
        <div class="row items-center justify-between q-mb-md">
          <div>
            <div class="text-h6">Prompt 段落</div>
            <div class="text-caption text-grey-7">{{ snapshot?.id }}</div>
          </div>
          <q-btn flat round icon="close" aria-label="关闭快照详情" @click="$emit('update:modelValue', false)" />
        </div>
        <q-expansion-item
          v-for="segment in segments"
          :key="`${segment.section}-${segment.source}`"
          expand-separator
          :label="segment.section"
          :caption="`${segment.tokens || 0} tokens · ${segment.source}`"
        >
          <pre class="memory-pre">{{ segment.preview || segment.content || '无预览' }}</pre>
        </q-expansion-item>
      </div>
    </q-scroll-area>
  </q-drawer>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { L0AssemblySegment, L0AssemblySnapshot } from '../../features/memory/types';

const props = defineProps<{
  modelValue: boolean;
  snapshot: L0AssemblySnapshot | null;
}>();

defineEmits<{
  'update:modelValue': [value: boolean];
}>();

const segments = computed(() =>
  props.snapshot ? parseJSON<L0AssemblySegment[]>(props.snapshot.segments_json, []) : [],
);

function parseJSON<T>(raw: string, fallback: T): T {
  try {
    const parsed = JSON.parse(raw || '');
    return parsed ?? fallback;
  } catch {
    return fallback;
  }
}
</script>
