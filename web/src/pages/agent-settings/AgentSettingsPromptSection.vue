<template>
  <section class="settings-section">
    <div class="section-heading">
      <div class="section-heading__main">
        <div class="section-title">
          <span class="section-title__text">系统提示模式</span>
        </div>
        <p class="settings-section__hint">控制运行时注入的提示块体量与人格强度。</p>
      </div>
    </div>
    <div class="prompt-mode-segment" role="radiogroup" aria-label="系统提示模式">
      <button
        v-for="mode in promptModes"
        :key="mode.value"
        type="button"
        role="radio"
        :aria-checked="systemPromptMode === mode.value"
        class="prompt-mode-segment__item"
        :class="{ 'is-active': systemPromptMode === mode.value }"
        @click="$emit('update:systemPromptMode', mode.value)"
      >
        <span class="prompt-mode-segment__label">{{ mode.label }}</span>
        <span class="prompt-mode-segment__tokens">{{ mode.tokens }}</span>
        <q-tooltip :delay="300">{{ mode.caption }}</q-tooltip>
      </button>
    </div>
    <p v-if="activeMode" class="prompt-mode-segment__caption">
      <q-icon name="info" size="14px" />
      <span>{{ activeMode.caption }}</span>
    </p>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps<{
  systemPromptMode: string;
  promptModes: { value: string; label: string; caption: string; tokens: string }[];
}>();

defineEmits<{ 'update:systemPromptMode': [value: string] }>();

const activeMode = computed(() => props.promptModes.find((m) => m.value === props.systemPromptMode));
</script>
