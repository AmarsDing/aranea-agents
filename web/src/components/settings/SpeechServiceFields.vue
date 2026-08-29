<template>
  <div class="speech-fields">
    <section class="settings-section settings-section--span">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <q-icon name="hearing" size="sm" color="primary" />
            <span class="section-title__text">{{ t('settingsPage.speech.asrTitle') }}</span>
          </div>
          <p class="settings-section__hint">{{ t('settingsPage.speech.asrHint') }}</p>
        </div>
        <q-badge :color="asrConfigured ? 'positive' : 'warning'">
          {{
            asrConfigured
              ? t('settingsPage.speech.asrConfigured')
              : t('settingsPage.speech.asrNotConfigured')
          }}
        </q-badge>
      </div>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-select
          v-model="form.asr_driver"
          class="app-glass-control app-field-sm"
          dense
          outlined
          emit-value
          map-options
          :label="t('settingsPage.speech.driver')"
          :options="driverOptions"
        />
        <q-select
          v-model="form.asr_language"
          class="app-glass-control app-field-sm"
          dense
          outlined
          emit-value
          map-options
          :label="t('settingsPage.speech.language')"
          :options="languageOptions"
        />
        <q-input
          v-model="form.asr_endpoint"
          class="app-glass-control app-grid-span-full app-field-long"
          dense
          outlined
          :label="t('settingsPage.speech.endpoint')"
          :hint="t('settingsPage.speech.endpointHint')"
        />
        <q-input
          v-model="form.asr_app_key"
          class="app-glass-control app-field-sm"
          dense
          outlined
          type="password"
          :label="t('settingsPage.speech.appKey')"
          :placeholder="asrHasApiKey ? t('settingsPage.speech.apiKeySet') : t('settingsPage.speech.apiKeyEmpty')"
        />
        <q-input
          v-model="form.asr_access_key"
          class="app-glass-control app-field-sm"
          dense
          outlined
          type="password"
          :label="t('settingsPage.speech.accessKey')"
          :placeholder="asrHasApiKey ? t('settingsPage.speech.apiKeySet') : t('settingsPage.speech.apiKeyEmpty')"
        />
        <q-input
          v-model="form.asr_resource_id"
          class="app-glass-control app-grid-span-full app-field-long"
          dense
          outlined
          :label="t('settingsPage.speech.resourceId')"
        />
      </div>
    </section>

    <section class="settings-section settings-section--span">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <q-icon name="record_voice_over" size="sm" color="primary" />
            <span class="section-title__text">{{ t('settingsPage.speech.ttsTitle') }}</span>
          </div>
          <p class="settings-section__hint">{{ t('settingsPage.speech.ttsHint') }}</p>
        </div>
        <q-badge :color="ttsConfigured ? 'positive' : 'warning'">
          {{
            ttsConfigured
              ? t('settingsPage.speech.ttsConfigured')
              : t('settingsPage.speech.ttsNotConfigured')
          }}
        </q-badge>
      </div>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-select
          v-model="form.tts_driver"
          class="app-glass-control app-field-sm"
          dense
          outlined
          emit-value
          map-options
          :label="t('settingsPage.speech.driver')"
          :options="driverOptions"
        />
        <q-input
          v-model.number="form.tts_speed_ratio"
          class="app-glass-control app-field-sm"
          dense
          outlined
          type="number"
          min="0.5"
          max="2"
          step="0.05"
          :label="t('settingsPage.speech.speedRatio')"
          :hint="t('settingsPage.speech.speedRatioHint')"
        />
        <q-input
          v-model="form.tts_endpoint"
          class="app-glass-control app-grid-span-full app-field-long"
          dense
          outlined
          :label="t('settingsPage.speech.endpoint')"
          :hint="t('settingsPage.speech.endpointHint')"
        />
        <q-input
          v-model="form.tts_voice"
          class="app-glass-control app-field-sm"
          dense
          outlined
          :label="t('settingsPage.speech.voice')"
          :hint="t('settingsPage.speech.voiceHint')"
        />
        <q-input
          v-model="form.tts_resource_id"
          class="app-glass-control app-field-sm"
          dense
          outlined
          :label="t('settingsPage.speech.resourceId')"
        />
        <q-input
          v-model="form.tts_app_key"
          class="app-glass-control app-field-sm"
          dense
          outlined
          type="password"
          :label="t('settingsPage.speech.appKey')"
          :placeholder="ttsHasApiKey ? t('settingsPage.speech.apiKeySet') : t('settingsPage.speech.apiKeyEmpty')"
        />
        <q-input
          v-model="form.tts_access_key"
          class="app-glass-control app-field-sm"
          dense
          outlined
          type="password"
          :label="t('settingsPage.speech.accessKey')"
          :placeholder="ttsHasApiKey ? t('settingsPage.speech.apiKeySet') : t('settingsPage.speech.apiKeyEmpty')"
        />
      </div>
    </section>

    <section class="settings-section settings-section--span">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <q-icon name="save" size="sm" color="primary" />
            <span class="section-title__text">{{ t('settingsPage.speech.archiveTitle') }}</span>
          </div>
          <p class="settings-section__hint">{{ t('settingsPage.speech.archiveHint') }}</p>
        </div>
      </div>
      <q-toggle v-model="form.archive_user_audio" :label="t('settingsPage.speech.archiveToggle')" />
    </section>
  </div>
</template>

<script setup lang="ts">
const form = defineModel<SpeechFormState>('form', { required: true });
import { useI18n } from 'vue-i18n';
import AppStatusChip from '../common/AppStatusChip.vue';
import {
  SPEECH_ASR_LANGUAGE_OPTIONS,
  SPEECH_DRIVER_OPTIONS,
  type SpeechFormState,
} from '../../features/system-settings/speech';

defineProps<{
  asrConfigured?: boolean;
  asrHasApiKey?: boolean;
  ttsConfigured?: boolean;
  ttsHasApiKey?: boolean;
}>();

const { t } = useI18n();
const driverOptions = SPEECH_DRIVER_OPTIONS;
const languageOptions = SPEECH_ASR_LANGUAGE_OPTIONS;
</script>
