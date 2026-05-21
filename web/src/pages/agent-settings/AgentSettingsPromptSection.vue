<template>
  <section class="settings-section">
    <div class="section-heading">
      <div class="section-title">
        <span class="section-title__text">系统提示模式</span>
      </div>
      <div class="text-caption text-grey-7">控制运行时注入的提示块体量与人格强度。</div>
    </div>
    <div class="app-form-field-grid">
      <q-card
        v-for="mode in promptModes"
        :key="mode.value"
        flat
        bordered
        class="prompt-mode-card cursor-pointer"
        :class="{ 'is-active': systemPromptMode === mode.value }"
        @click="$emit('update:systemPromptMode', mode.value)"
      >
        <q-card-section>
          <div class="text-subtitle2 text-weight-bold">{{ mode.label }}</div>
          <div class="text-caption text-grey-7 q-mt-xs">{{ mode.caption }}</div>
          <q-chip dense square class="prompt-mode-card__token q-mt-sm">{{ mode.tokens }}</q-chip>
        </q-card-section>
      </q-card>
    </div>
  </section>
</template>

<script setup lang="ts">
defineProps<{
  systemPromptMode: string;
  promptModes: { value: string; label: string; caption: string; tokens: string }[];
}>();

defineEmits<{ "update:systemPromptMode": [value: string] }>();
</script>
