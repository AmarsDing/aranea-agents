<template>
  <div :class="['range-slider', { 'range-slider--disabled': disabled }]">
    <label v-if="label" class="range-slider__label">{{ label }}</label>
    <div class="range-slider__row">
      <q-slider
        :model-value="modelValue"
        :min="min"
        :max="max"
        :step="step"
        :disable="disabled"
        label-always
        class="range-slider__slider"
        @update:model-value="onInput"
      />
      <span class="range-slider__value">{{ modelValue }}</span>
    </div>
    <div v-if="hint" class="range-slider__hint">{{ hint }}</div>
  </div>
</template>

<script setup lang="ts">
const props = withDefaults(
  defineProps<{
    modelValue: number;
    min?: number;
    max?: number;
    step?: number;
    label?: string;
    hint?: string;
    disabled?: boolean;
  }>(),
  {
    min: 0,
    max: 100,
    step: 1,
    label: '',
    hint: '',
    disabled: false,
  },
);

const emit = defineEmits<{
  'update:modelValue': [value: number];
}>();

function onInput(val: number | null) {
  if (val !== null) {
    const clamped = Math.min(props.max, Math.max(props.min, val));
    emit('update:modelValue', clamped);
  }
}
</script>

<style lang="scss">
.range-slider {
  // Use theme tokens so colors adapt to light/dark automatically (S9 fix).
  --rs-label-color: var(--color-text-secondary);
  --rs-hint-color: var(--color-text-tertiary);
  --rs-value-color: var(--color-text-primary);

  &__label {
    display: block;
    font-size: 12px;
    color: var(--rs-label-color);
    margin-bottom: 4px;
    line-height: 1;
  }

  &__row {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  &__slider {
    flex: 1;
  }

  &__value {
    flex-shrink: 0;
    min-width: 40px;
    text-align: center;
    font-size: 13px;
    font-weight: 500;
    color: var(--rs-value-color);
  }

  &__hint {
    font-size: 12px;
    color: var(--rs-hint-color);
    margin-top: 2px;
    line-height: 1.3;
  }

  &--disabled {
    opacity: 0.6;
    pointer-events: none;
  }
}
</style>
