<template>
  <section class="settings-section">
    <div class="section-heading">
      <div>
        <div class="text-subtitle1 text-weight-bold">系统提示模式</div>
        <div class="text-caption text-grey-7">控制运行时注入的提示块体量与人格强度。</div>
      </div>
    </div>
    <div class="row q-col-gutter-md">
      <div v-for="mode in promptModes" :key="mode.value" class="col-12 col-md-3">
        <q-card
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
