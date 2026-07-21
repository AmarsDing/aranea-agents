// Container: approved — platform memory policy toggles (persisted to system_settings). // FD4+FB3 fix: data
fetching/saving + error handling extracted to useMemoryPlatformSettings composable.
<template>
  <q-card flat bordered class="memory-card">
    <q-card-section class="row items-center justify-between">
      <div>
        <div class="text-h6">{{ t('memory.platformSettings.title') }}</div>
        <div class="text-caption text-grey-7">
          {{ t('memory.platformSettings.subtitle') }}
        </div>
      </div>
      <q-btn flat dense icon="refresh" :loading="loading" @click="load" />
    </q-card-section>

    <q-card-section v-if="loaded" class="column q-gutter-md">
      <q-toggle
        v-model="form.policy_strict"
        color="warning"
        :label="t('memory.platformSettings.policyStrict')"
        :disable="envPolicyStrict"
      />
      <div v-if="envPolicyStrict" class="text-caption text-orange-8">
        {{ t('memory.platformSettings.policyStrictEnvHint') }}
      </div>

      <q-toggle
        v-model="form.episode_backfill_disabled"
        color="primary"
        :label="t('memory.platformSettings.backfillDisabled')"
        :disable="envBackfillDisabled"
      />
      <div v-if="envBackfillDisabled" class="text-caption text-orange-8">
        {{ t('memory.platformSettings.backfillDisabledEnvHint') }}
      </div>

      <div class="row q-gutter-sm">
        <q-btn color="primary" :label="t('memory.platformSettings.save')" :loading="saving" @click="save" />
      </div>

      <q-banner v-if="message" rounded :class="messageOk ? 'bg-positive text-white' : 'bg-negative text-white'">
        {{ message }}
      </q-banner>
    </q-card-section>

    <q-card-section v-else-if="!loading" class="text-grey-7 text-caption">{{
      t('memory.platformSettings.loadFailed')
    }}</q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { useMemoryPlatformSettings } from './composables/useMemoryPlatformSettings';

const { t } = useI18n();

const { loading, saving, loaded, envPolicyStrict, envBackfillDisabled, message, messageOk, form, load, save } =
  useMemoryPlatformSettings();
</script>
