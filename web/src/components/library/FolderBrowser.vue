<template>
  <!-- 一层一层选文件夹（建库向导选本机文件夹、移动文件选目标目录共用；只管展示，数据由上层给） -->
  <div class="lb-fb">
    <div class="lb-fb__bar">
      <q-btn flat dense no-caps icon="arrow_upward" :disable="!canUp || loading" :label="upLabel" @click="emit('up')" />
      <span class="lb-fb__path lb-mono" :title="path">{{ path }}</span>
    </div>
    <div ref="listEl" class="lb-fb__list" role="listbox" :aria-label="path" :aria-busy="loading">
      <q-linear-progress v-if="loading" indeterminate size="2px" color="primary" />
      <button
        v-for="d in dirs"
        :key="d.path"
        type="button"
        role="option"
        class="lb-fb__item"
        :aria-selected="false"
        @click="emit('open', d.path)"
      >
        <q-icon name="folder" size="18px" aria-hidden="true" />
        <span>{{ d.name }}</span>
        <q-icon name="chevron_right" size="16px" class="lb-fb__go" aria-hidden="true" />
      </button>
      <p v-if="!loading && !dirs.length" class="lb-hint lb-fb__empty">{{ emptyText }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';

const props = defineProps<{
  /** 当前位置（展示用） */
  path: string;
  dirs: Array<{ name: string; path: string }>;
  canUp: boolean;
  loading: boolean;
  upLabel: string;
  emptyText: string;
}>();

const emit = defineEmits<{ open: [path: string]; up: [] }>();
const listEl = ref<HTMLElement | null>(null);

// 换了一层就回到列表顶部
watch(
  () => props.path,
  () => listEl.value?.scrollTo({ top: 0 }),
);
</script>
