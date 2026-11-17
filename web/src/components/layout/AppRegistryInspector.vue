<template>
  <div
    v-if="modelValue"
    class="app-registry-inspector"
    :data-persistent="persistent"
    :class="{ 'app-registry-inspector--overlay': !persistent }"
  >
    <button
      v-if="!persistent"
      type="button"
      class="app-registry-inspector__scrim"
      tabindex="-1"
      :aria-label="t('shell.common.close')"
      @click="close"
    />
    <aside
      ref="panel"
      class="app-registry-inspector__panel app-surface-l3"
      :role="persistent ? 'complementary' : 'dialog'"
      :aria-modal="persistent ? undefined : 'true'"
      :aria-label="title"
      tabindex="-1"
      @keydown.esc.stop.prevent="close"
    >
      <header class="app-registry-inspector__header">
        <div class="min-width-0">
          <div v-if="eyebrow" class="app-registry-inspector__eyebrow">{{ eyebrow }}</div>
          <h2 class="app-registry-inspector__title">{{ title }}</h2>
        </div>
        <q-btn
          flat
          dense
          round
          icon="close"
          :aria-label="t('shell.common.close')"
          @click="close"
        />
      </header>
      <div class="app-registry-inspector__body">
        <slot />
      </div>
      <footer v-if="$slots.footer" class="app-registry-inspector__footer">
        <slot name="footer" />
      </footer>
    </aside>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';

const props = defineProps<{
  modelValue: boolean;
  title: string;
  eyebrow?: string;
}>();
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>();
const { t } = useI18n();
const $q = useQuasar();
const panel = ref<HTMLElement | null>(null);
const persistent = computed(() => $q.screen.width >= 1440);
let opener: HTMLElement | null = null;

function close() {
  emit('update:modelValue', false);
}

watch(
  () => props.modelValue,
  (open, wasOpen) => {
    if (open && !wasOpen) {
      const active = document.activeElement;
      opener = active instanceof HTMLElement && !active.classList.contains('q-focus-helper') ? active : null;
      void nextTick(() => panel.value?.focus());
    } else if (!open && wasOpen) {
      void nextTick(() => {
        if (opener?.isConnected) opener.focus();
        opener = null;
      });
    }
  },
  { immediate: true },
);
</script>
