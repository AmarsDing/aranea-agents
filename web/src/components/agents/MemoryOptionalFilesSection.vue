<template>
  <section
    v-if="availableOptionalFiles.length"
    class="settings-section"
    :class="{ 'settings-section--disabled': disabled }"
  >
    <div class="section-heading">
      <div class="section-heading__main">
        <div class="section-title">
          <span class="section-title__text">可选文件</span>
        </div>
        <p class="settings-section__hint">按需添加可选 Prompt 文件到 Agent 配置。</p>
      </div>
    </div>
    <div class="row q-gutter-xs">
      <q-btn
        v-for="file in availableOptionalFiles"
        :key="file.name"
        dense
        outline
        rounded
        color="primary"
        icon="add"
        :label="`添加 ${file.name}`"
        :disable="disabled"
        @click="$emit('add-optional-file', file.name)"
      />
    </div>
  </section>
</template>

<script setup lang="ts">
import type { AgentFile } from './agentUi';

withDefaults(
  defineProps<{
    availableOptionalFiles?: AgentFile[];
    disabled?: boolean;
  }>(),
  {
    availableOptionalFiles: () => [],
    disabled: false,
  },
);

defineEmits<{
  'add-optional-file': [name: string];
}>();
</script>
