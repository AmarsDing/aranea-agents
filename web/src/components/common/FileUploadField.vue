<template>
  <div :class="['file-upload-field', { 'file-upload-field--disabled': disabled }]">
    <label v-if="label" class="file-upload-field__label">{{ label }}</label>
    <div class="file-upload-field__row">
      <q-input
        :model-value="modelValue"
        dense
        outlined
        :disable="disabled"
        class="file-upload-field__input"
        @update:model-value="(v: string | number | null) => emit('update:modelValue', String(v ?? ''))"
      />
      <q-btn
        flat
        dense
        no-caps
        icon="attach_file"
        label="浏览"
        :disable="disabled"
        class="file-upload-field__btn"
        @click="triggerFilePicker"
      />
      <input
        ref="fileInputRef"
        type="file"
        :accept="accept"
        class="file-upload-field__hidden"
        @change="onFileSelected"
      />
    </div>
    <div v-if="hint" class="file-upload-field__hint">{{ hint }}</div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';

const props = withDefaults(
  defineProps<{
    modelValue: string;
    label?: string;
    hint?: string;
    accept?: string;
    disabled?: boolean;
  }>(),
  {
    label: '',
    hint: '',
    accept: '*',
    disabled: false,
  },
);

const emit = defineEmits<{
  'update:modelValue': [value: string];
}>();

const fileInputRef = ref<HTMLInputElement | null>(null);

function triggerFilePicker() {
  fileInputRef.value?.click();
}

function onFileSelected(event: Event) {
  const target = event.target as HTMLInputElement;
  const file = target.files?.[0];
  if (file) {
    emit('update:modelValue', file.name);
  }
  // Reset so the same file can be re-selected
  target.value = '';
}
</script>

<style lang="scss">
.file-upload-field {
  &__label {
    display: block;
    font-size: 12px;
    color: #666;
    margin-bottom: 4px;
    line-height: 1;
  }

  &__row {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  &__input {
    flex: 1;
  }

  &__btn {
    flex-shrink: 0;
  }

  &__hint {
    font-size: 12px;
    color: #999;
    margin-top: 2px;
    line-height: 1.3;
  }

  &__hidden {
    display: none;
  }

  &--disabled {
    opacity: 0.6;
    pointer-events: none;
  }
}
</style>
