<template>
  <q-page class="flex flex-center q-pa-md">
    <div class="text-center">
      <q-icon name="phone_iphone" size="48px" class="text-grey-5 q-mb-md" />
      <div class="text-h6 text-grey-7">{{ t('mobile.placeholderTitle') }}</div>
      <div class="text-body2 text-grey-6 q-mt-sm">{{ t('mobile.placeholderBody') }}</div>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';

const { t } = useI18n();
</script>
