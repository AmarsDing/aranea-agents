<template>
  <q-dialog v-model="open">
    <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-start justify-between no-wrap">
        <div class="col min-width-0">
          <div class="app-glass-dialog__title">{{ t('mobile.pairingTitle') }}</div>
          <div class="app-glass-dialog__subtitle">{{ t('mobile.pairingSubtitle') }}</div>
        </div>
        <q-btn v-close-popup flat dense round icon="close" />
      </q-card-section>
      <q-separator />
      <div class="app-glass-dialog__scroll">
        <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md">
          <q-input
            v-model="url"
            outlined
            :label="t('mobile.pairingUrlLabel')"
            :placeholder="t('mobileServer.urlPlaceholder')"
            :error="showError"
            :error-message="t('mobileServer.urlInvalid')"
            autocomplete="url"
            inputmode="url"
            @update:model-value="showError = false"
          />
          <div class="pairing-qr-frame flex flex-center">
            <img v-if="qrDataUrl" :src="qrDataUrl" :alt="t('mobile.pairingTitle')" class="pairing-qr-image" />
            <div v-else class="text-body2 text-grey-7 text-center q-pa-md">
              {{ t('mobile.pairingQrHint') }}
            </div>
          </div>
        </q-card-section>
      </div>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import QRCode from 'qrcode';
import { buildPairingQrPayload, isPlausibleServerUrl } from '../../features/mobile/pairingQr';

const STORAGE_KEY = 'aranea.mobilePairingUrl';

const open = defineModel<boolean>({ required: true });

const { t } = useI18n();

const url = ref(localStorage.getItem(STORAGE_KEY) ?? '');
const showError = ref(false);
const qrDataUrl = ref('');

// Regenerate the QR whenever the entered address becomes (in)valid. The last
// valid address is remembered so the dialog reopens ready to scan.
watch(
  url,
  (value) => {
    const trimmed = value.trim();
    if (!isPlausibleServerUrl(trimmed)) {
      qrDataUrl.value = '';
      return;
    }
    localStorage.setItem(STORAGE_KEY, trimmed);
    QRCode.toDataURL(buildPairingQrPayload(trimmed), { width: 240, margin: 1 })
      .then((dataUrl) => {
        // Guard against out-of-order resolves after further edits.
        if (url.value.trim() === trimmed) qrDataUrl.value = dataUrl;
      })
      .catch(() => {
        qrDataUrl.value = '';
      });
  },
  { immediate: true },
);
</script>

<style lang="sass" scoped>
.pairing-qr-frame
  min-height: 240px
  border-radius: 16px
  border: 1px solid var(--glass-border)

.pairing-qr-image
  width: 240px
  height: 240px
  border-radius: 8px
</style>
