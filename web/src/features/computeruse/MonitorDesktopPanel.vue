<template>
  <q-card flat bordered class="monitor-card cu-monitor">
    <q-card-section class="row items-center no-wrap">
      <q-icon name="desktop_windows" size="20px" class="cu-monitor__head-icon" />
      <div class="q-ml-sm">
        <div class="text-h6 text-weight-bold">{{ t('computeruse.monitor.title') }}</div>
        <div class="text-caption text-grey-7 q-mt-xs">{{ t('computeruse.monitor.hint') }}</div>
      </div>
    </q-card-section>
    <q-separator />
    <q-card-section>
      <div class="cu-monitor__row">
        <q-input
          v-model="sessionInput"
          dense
          outlined
          clearable
          class="cu-monitor__input"
          :placeholder="t('computeruse.monitor.sessionPlaceholder')"
          @keyup.enter="applySession"
        >
          <template #prepend>
            <q-icon name="tag" size="16px" />
          </template>
        </q-input>
        <q-btn
          unelevated
          rounded
          no-caps
          class="app-accent-btn"
          icon="play_arrow"
          :label="t('computeruse.monitor.load')"
          @click="applySession"
        />
      </div>
      <CuStepStream v-if="sessionId" :session-id="sessionId" readonly class="q-mt-md" />
    </q-card-section>
  </q-card>
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

<style scoped>
.cu-monitor__head-icon {
  color: var(--color-accent);
}

.cu-monitor__row {
  display: flex;
  gap: 8px;
  align-items: center;
}

.cu-monitor__input {
  flex: 1;
  min-width: 0;
}
</style>
