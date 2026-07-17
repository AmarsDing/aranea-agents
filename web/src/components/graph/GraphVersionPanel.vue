<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="graph-version-panel app-dialog-card app-dialog-card--sm app-glass-dialog">
      <q-card-section class="app-glass-dialog__head">
        <div class="app-glass-dialog__title">{{ t('graphs.versionPanelTitle') }}</div>
        <q-space />
        <q-btn flat dense round icon="close" @click="$emit('update:modelValue', false)" />
      </q-card-section>
      <q-separator />
      <q-card-section class="app-dialog-body app-glass-dialog__body">
        <div v-if="loading" class="row justify-center q-py-lg">
          <q-spinner color="primary" size="28px" />
        </div>
        <div v-else-if="!versions.length" class="text-caption text-grey-7 text-center q-py-lg">
          {{ t('graphs.versionPanelEmpty') }}
        </div>
        <q-list v-else bordered separator class="rounded-borders">
          <q-item v-for="item in versions" :key="item.version">
            <q-item-section>
              <q-item-label> v{{ item.version }} · {{ item.name || t('graphs.versionPanelUnnamed') }} </q-item-label>
              <q-item-label caption>{{ formatTime(item.savedAt) }}</q-item-label>
            </q-item-section>
            <q-item-section side>
              <q-btn
                flat
                dense
                color="primary"
                :label="t('graphs.versionPanelRollback')"
                :loading="rollingBackVersion === item.version"
                @click="confirmRollback(item.version)"
              />
            </q-item-section>
          </q-item>
        </q-list>
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import type { GraphVersionInfo } from '../../features/graph/types';
import { formatTime } from '../../features/graph/utils';

const { t } = useI18n();
const $q = useQuasar();

defineProps<{
  modelValue: boolean;
  versions: GraphVersionInfo[];
  loading: boolean;
  rollingBackVersion: number | null;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  rollback: [version: number];
}>();

function confirmRollback(version: number) {
  $q.dialog({
    title: t('graphs.versionPanelConfirmTitle'),
    message: t('graphs.versionPanelConfirmMessage', { version }),
    cancel: true,
    persistent: true,
  }).onOk(() => {
    emit('rollback', version);
  });
}
</script>
