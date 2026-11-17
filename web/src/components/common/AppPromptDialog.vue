<template>
  <AppDialog
    :model-value="modelValue"
    :title="title"
    size="sm"
    :persistent="persistent"
    :loading="loading"
    @update:model-value="onVisibleChange"
  >
    <div class="app-prompt-dialog">
      <p v-if="message" class="app-prompt-dialog__message">{{ message }}</p>

      <q-input
        v-if="type !== 'radio'"
        ref="inputRef"
        v-model="draft"
        class="app-prompt-dialog__field"
        :type="type === 'textarea' ? 'textarea' : 'text'"
        :label="label || undefined"
        :placeholder="placeholder || undefined"
        :autogrow="type === 'textarea'"
        :error="showRequiredError"
        :error-message="showRequiredError ? t('common.required') : undefined"
        outlined
        dense
        autofocus
        data-test="prompt-input"
        @keyup.enter="type === 'text' && submit()"
      />

      <div v-else class="app-prompt-dialog__options" role="radiogroup" :aria-label="title">
        <label
          v-for="item in items"
          :key="item.value"
          class="app-prompt-dialog__option"
          :class="{ 'app-prompt-dialog__option--disabled': item.disabled }"
        >
          <input v-model="draft" type="radio" :value="item.value" :disabled="item.disabled" data-test="prompt-option" />
          <span class="app-prompt-dialog__option-copy">
            <span class="app-prompt-dialog__option-label">{{ item.label }}</span>
            <span v-if="item.hint" class="app-prompt-dialog__option-hint">{{ item.hint }}</span>
          </span>
        </label>
      </div>
    </div>

    <template #actions>
      <q-btn flat no-caps :label="cancelLabel || t('common.cancel')" :disable="loading" @click="cancel" />
      <q-btn
        unelevated
        no-caps
        :color="tone === 'danger' ? 'negative' : 'primary'"
        :label="confirmLabel || t('common.confirm')"
        :disable="submitDisabled"
        :loading="loading"
        data-test="prompt-submit"
        @click="submit"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import AppDialog from './AppDialog.vue';
import type { AppPromptItem, AppPromptType } from '../../composables/useAppPrompt';

const props = withDefaults(
  defineProps<{
    modelValue: boolean;
    title: string;
    message?: string;
    label?: string;
    placeholder?: string;
    type?: AppPromptType;
    items?: AppPromptItem[];
    defaultValue?: string;
    required?: boolean;
    confirmLabel?: string;
    cancelLabel?: string;
    tone?: 'primary' | 'danger';
    persistent?: boolean;
    loading?: boolean;
  }>(),
  {
    message: '',
    label: '',
    placeholder: '',
    type: 'text',
    items: () => [],
    defaultValue: '',
    required: false,
    confirmLabel: '',
    cancelLabel: '',
    tone: 'primary',
    persistent: true,
    loading: false,
  },
);

const emit = defineEmits<{ 'update:modelValue': [value: boolean]; submit: [value: string] }>();
const { t } = useI18n();
const draft = ref(props.defaultValue);
const inputRef = ref<{ focus: () => void } | null>(null);
const touched = ref(false);

// 每次打开都用最新初值重置：同一实例会被串行复用（例如 409 后带预设名重开）。
watch(
  () => props.modelValue,
  (visible) => {
    if (!visible) return;
    draft.value = props.defaultValue;
    touched.value = false;
  },
  { immediate: true },
);

const trimmed = computed(() => draft.value.trim());
const isEmpty = computed(() => (props.type === 'radio' ? !draft.value : !trimmed.value));
const showRequiredError = computed(() => props.required && touched.value && isEmpty.value);
/** 文本类始终可点：必填为空时点击给出内联错误，而不是一个点了没反应的死按钮。 */
const submitDisabled = computed(() => props.loading || (props.type === 'radio' && !draft.value));

function submit() {
  if (props.loading) return;
  if (props.type === 'radio' ? !draft.value : props.required && isEmpty.value) {
    touched.value = true;
    return;
  }
  emit('submit', props.type === 'radio' ? draft.value : trimmed.value);
}

function cancel() {
  emit('update:modelValue', false);
}

function onVisibleChange(visible: boolean) {
  if (!visible && props.loading) return;
  emit('update:modelValue', visible);
}
</script>
