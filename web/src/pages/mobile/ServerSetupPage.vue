<template>
  <q-page class="flex flex-center q-pa-md">
    <q-card flat bordered class="server-setup-card">
      <q-card-section>
        <div class="text-h6">{{ $t('mobileServer.setupTitle') }}</div>
        <div class="text-body2 text-grey-7 q-mt-xs">{{ $t('mobileServer.setupSubtitle') }}</div>
      </q-card-section>

      <q-card-section class="q-gutter-md">
        <q-input
          v-model="url"
          outlined
          :label="$t('mobileServer.urlLabel')"
          :placeholder="$t('mobileServer.urlPlaceholder')"
          :error="showError"
          :error-message="$t('mobileServer.urlInvalid')"
          autocomplete="url"
          inputmode="url"
          @update:model-value="showError = false"
        />
        <div v-if="statusMessage" class="text-body2" :class="statusOk ? 'text-positive' : 'text-negative'">
          {{ statusMessage }}
        </div>
      </q-card-section>

      <q-card-actions align="right" class="q-pa-md">
        <q-btn flat :label="$t('mobileServer.test')" :loading="testing" @click="onTest" />
        <q-btn color="primary" unelevated :label="$t('mobileServer.save')" :loading="saving" @click="onSave" />
      </q-card-actions>
    </q-card>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { fetchBackendConfig, isPlausibleServerUrl, probeBackend, saveBackendConfig } from 'src/services/backendConfig';

const router = useRouter();
const { t } = useI18n();

const url = ref('');
const showError = ref(false);
const testing = ref(false);
const saving = ref(false);
const statusMessage = ref('');
const statusOk = ref(false);

onMounted(async () => {
  const cfg = await fetchBackendConfig();
  if (cfg?.url) url.value = cfg.url;
});

function validate(): boolean {
  const ok = isPlausibleServerUrl(url.value);
  showError.value = !ok;
  return ok;
}

async function onTest() {
  statusMessage.value = '';
  if (!validate()) return;
  testing.value = true;
  try {
    // Save first so the proxy targets the new upstream, then probe through it.
    const saved = await saveBackendConfig(url.value.trim());
    if (!saved.ok) {
      statusOk.value = false;
      statusMessage.value = t('mobileServer.testFailed');
      return;
    }
    const reachable = await probeBackend();
    statusOk.value = reachable;
    statusMessage.value = reachable ? t('mobileServer.testOk') : t('mobileServer.testFailed');
  } finally {
    testing.value = false;
  }
}

async function onSave() {
  statusMessage.value = '';
  if (!validate()) return;
  saving.value = true;
  try {
    const saved = await saveBackendConfig(url.value.trim());
    if (!saved.ok) {
      statusOk.value = false;
      statusMessage.value = t('mobileServer.testFailed');
      return;
    }
    const reachable = await probeBackend();
    if (!reachable) {
      statusOk.value = false;
      statusMessage.value = t('mobileServer.testFailed');
      return;
    }
    await router.push('/login');
  } finally {
    saving.value = false;
  }
}
</script>

<style lang="sass" scoped>
.server-setup-card
  width: 100%
  max-width: 420px
</style>
