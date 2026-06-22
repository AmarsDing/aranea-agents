<template>
  <transition name="hint-fade">
    <div v-if="hint" class="skill-hint-bar q-px-md q-py-xs">
      <q-icon name="tips_and_updates" size="xs" class="q-mr-xs text-warning" />
      <span class="text-caption">
        {{ t('chat.skillHint.detected') }}: <strong>{{ hint.matched_skill }}</strong>
        <span v-if="hint.trigger" class="text-grey">（{{ t('chat.skillHint.match') }}: {{ hint.trigger }}）</span>
      </span>
      <q-btn flat dense size="xs" :label="t('chat.skillHint.load')" color="accent" class="q-ml-sm" @click="onLoad" />
      <q-btn flat dense round size="xs" icon="close" class="q-ml-xs" @click="onDismiss" />
    </div>
  </transition>
</template>

<script setup lang="ts">
import { watch, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { SkillHint } from '../../features/skills/types';

const { t } = useI18n();

const HINT_AUTO_DISMISS_MS = 3000;

const props = defineProps<{
  hint: SkillHint | null;
}>();

const emit = defineEmits<{
  loadSkill: [slug: string];
  dismiss: [];
}>();

const dismissTimer = ref<ReturnType<typeof setTimeout> | null>(null);

watch(
  () => props.hint,
  (h) => {
    if (dismissTimer.value !== null) {
      clearTimeout(dismissTimer.value);
      dismissTimer.value = null;
    }
    if (h) {
      dismissTimer.value = setTimeout(() => {
        emit('dismiss');
        dismissTimer.value = null;
      }, HINT_AUTO_DISMISS_MS);
    }
  },
  { immediate: true },
);

function onLoad() {
  if (props.hint) {
    emit('loadSkill', props.hint.matched_skill);
    emit('dismiss');
  }
}

function onDismiss() {
  emit('dismiss');
}
</script>

<style scoped>
.skill-hint-bar {
  display: flex;
  align-items: center;
  background: var(--q-warning);
  border-radius: 4px;
  min-height: 28px;
}

.hint-fade-enter-active,
.hint-fade-leave-active {
  transition: opacity 0.3s ease;
}
.hint-fade-enter-from,
.hint-fade-leave-to {
  opacity: 0;
}
</style>
