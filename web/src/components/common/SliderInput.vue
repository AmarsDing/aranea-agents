<template>
  <div :class="['slider-input', { 'slider-input--dark': isDark, 'slider-input--disable': disable }]">
    <label v-if="label" class="slider-input__label">{{ label }}</label>
    <div class="slider-input__row">
      <q-slider
        :model-value="modelValue"
        :min="min"
        :max="max"
        :step="step"
        :disable="disable"
        label
        label-always
        class="slider-input__slider"
        @update:model-value="onSliderChange"
      />
      <q-input
        :model-value="inputDisplay"
        type="number"
        dense
        outlined
        :disable="disable"
        :rules="rules"
        class="slider-input__input"
        @update:model-value="onInputChange"
        @blur="onInputBlur"
      />
    </div>
    <div v-if="hint" class="slider-input__hint">{{ hint }}</div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useQuasar } from 'quasar';

const props = withDefaults(
  defineProps<{
    modelValue: number;
    min?: number;
    max?: number;
    step?: number;
    label?: string;
    hint?: string;
    disable?: boolean;
    rules?: Array<(val: string | number | null | undefined) => boolean | string>;
  }>(),
  {
    min: 0,
    max: 100,
    step: 1,
    label: '',
    hint: '',
    disable: false,
    rules: () => [],
  },
);

const emit = defineEmits<{
  'update:modelValue': [value: number];
}>();

const $q = useQuasar();
const isDark = computed(() => $q.dark.isActive);

// Track the raw input value so user can type freely before committing
const localInput = ref<string | null>(null);

const inputDisplay = computed(() => {
  if (localInput.value !== null) return localInput.value;
  return String(props.modelValue);
});

function clamp(val: number): number {
  return Math.min(props.max, Math.max(props.min, val));
}

function onSliderChange(val: number | null) {
  localInput.value = null;
  if (val !== null) {
    emit('update:modelValue', clamp(val));
  }
}

function onInputChange(v: string | number | null) {
  localInput.value = v !== null && v !== undefined ? String(v) : '';
  const num = Number(v);
  if (!Number.isNaN(num)) {
    emit('update:modelValue', clamp(num));
  }
}

function onInputBlur() {
  // Reset local input to reflect the clamped model value
  localInput.value = null;
}
</script>

<style lang="scss">
.slider-input {
  --si-label-color: #666;
  --si-hint-color: #999;
  --si-track-color: #e0e0e0;

  &--dark {
    --si-label-color: #aaa;
    --si-hint-color: #777;
    --si-track-color: #444;
  }

  &__label {
    display: block;
    font-size: 12px;
    color: var(--si-label-color);
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

  &__input {
    width: 80px;
    flex-shrink: 0;

    .q-field__control {
      min-height: 32px;
      height: 32px;
    }

    .q-field__marginal {
      height: 32px;
    }

    input {
      text-align: center;
      padding: 0 4px;
    }

    /* Hide spin buttons */
    input[type='number']::-webkit-inner-spin-button,
    input[type='number']::-webkit-outer-spin-button {
      -webkit-appearance: none;
      margin: 0;
    }
    input[type='number'] {
      -moz-appearance: textfield;
    }
  }

  &__hint {
    font-size: 12px;
    color: var(--si-hint-color);
    margin-top: 2px;
    line-height: 1.3;
  }

  &--disable {
    opacity: 0.6;
    pointer-events: none;
  }
}
</style>
