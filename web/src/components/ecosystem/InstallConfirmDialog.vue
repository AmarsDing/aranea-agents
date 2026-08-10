<template>
  <q-dialog :model-value="open" @update:model-value="emit('update:open', $event)">
    <q-card class="app-dialog-card app-glass-dialog install-confirm">
      <q-card-section class="app-glass-dialog__head row items-center justify-between">
        <div class="app-glass-dialog__title">{{ t('shopPage.installConfirmTitle') }}</div>
        <q-btn v-close-popup flat round dense icon="close" />
      </q-card-section>
      <q-separator />
      <q-card-section class="app-glass-dialog__body">
        <div class="text-body2 q-mb-md">
          {{ t('shopPage.installConfirmDesc', { name: asset.name }) }}
        </div>
        <div v-if="hasHighRisk" class="install-confirm__warning row items-start q-gutter-sm q-mb-md">
          <q-icon name="warning_amber" size="18px" color="warning" class="q-mt-xs" />
          <span class="text-body2">{{ t('shopPage.installConfirmHighRisk') }}</span>
        </div>
        <permission-list :permissions="asset.permissions" />
      </q-card-section>
      <q-separator />
      <q-card-actions align="right" class="app-glass-dialog__actions">
        <q-btn v-close-popup flat no-caps :label="t('common.cancel')" />
        <q-btn
          color="primary"
          unelevated
          no-caps
          :label="t('shopPage.installConfirmOk')"
          :loading="installing"
          @click="emit('confirm')"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { MarketAsset } from '../../features/ecosystem/types';
import PermissionList from './PermissionList.vue';

const props = defineProps<{
  open: boolean;
  asset: MarketAsset;
  installing?: boolean;
}>();

const emit = defineEmits<{
  'update:open': [value: boolean];
  confirm: [];
}>();

const { t } = useI18n();
const hasHighRisk = computed(() => props.asset.permissions.some((p) => p.risk === 'high'));
</script>

<style scoped>
.install-confirm {
  width: min(560px, 92vw);
  max-width: 92vw;
}

.install-confirm__warning {
  padding: 10px 12px;
  border-radius: 10px;
  background: rgb(240 155 84 / 12%);
  color: var(--color-warning);
}
</style>
