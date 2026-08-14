<template>
  <div class="cu-monitor">
    <p class="cu-monitor__hint">{{ t('computeruse.monitor.hint') }}</p>
    <div class="cu-monitor__row">
      <input
        v-model="sessionInput"
        class="cu-monitor__input"
        :placeholder="t('computeruse.monitor.sessionPlaceholder')"
        @keyup.enter="applySession"
      />
      <button class="cu-monitor__btn" type="button" @click="applySession">
        {{ t('computeruse.monitor.load') }}
      </button>
    </div>
    <CuStepStream v-if="sessionId" :session-id="sessionId" readonly />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRoute } from 'vue-router';
import CuStepStream from './CuStepStream.vue';

const { t } = useI18n();
const route = useRoute();
const initial = String(route.query.cu_session || '').trim();
const sessionInput = ref(initial);
const sessionId = ref(initial);

function applySession() {
  sessionId.value = sessionInput.value.trim();
}
</script>

<style lang="sass" scoped>
.cu-monitor
  display: flex
  flex-direction: column
  gap: 12px

  &__hint
    margin: 0
    font-size: 13px
    color: var(--color-text-secondary)

  &__row
    display: flex
    gap: 8px

  &__input
    flex: 1
    min-width: 0
    padding: 8px 10px
    border-radius: 8px
    border: 1px solid var(--glass-border)
    background: var(--glass-surface)
    color: var(--color-text-primary)

  &__btn
    padding: 8px 14px
    border: none
    border-radius: 8px
    font-weight: 600
    cursor: pointer
    background: var(--color-primary)
    color: var(--color-on-accent, #fff)
</style>
