<template>
  <div v-if="visible" class="chat-enqueue row items-end no-wrap q-gutter-x-sm q-mt-sm">
    <q-input
      v-model="draft"
      dense
      outlined
      autogrow
      class="col chat-enqueue__input"
      :dark="isDark"
      :disable="disabled"
      :placeholder="t('chat.enqueuePlaceholder', 'Message will be injected after the current tool step')"
      @keydown.enter.exact.prevent="submit"
    />
    <q-btn
      flat
      dense
      color="primary"
      icon="playlist_add"
      :disable="disabled || !draft.trim()"
      :aria-label="t('chat.enqueueSend', 'Enqueue message')"
      @click="submit"
    >
      <q-tooltip>{{ t("chat.enqueueHint", "Injected at the next safe tool boundary") }}</q-tooltip>
    </q-btn>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from "vue";
import { useI18n } from "vue-i18n";

const props = defineProps<{
  visible?: boolean;
  disabled?: boolean;
  isDark?: boolean;
}>();

const emit = defineEmits<{ enqueue: [content: string] }>();

const { t } = useI18n();
const draft = ref("");

const visible = computed(() => props.visible !== false);

function submit() {
  const text = draft.value.trim();
  if (!text || props.disabled) return;
  emit("enqueue", text);
  draft.value = "";
}
</script>

<style scoped lang="sass">
.chat-enqueue__input
  :deep(.q-field__control)
    border-radius: 12px
</style>
