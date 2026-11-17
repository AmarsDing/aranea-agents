<template>
  <AppDialog
    :model-value="modelValue"
    :title="t('graphs.versionPanelTitle')"
    size="sm"
    persistent
    @update:model-value="$emit('update:modelValue', $event)"
  >
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
  </AppDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { confirm } from '../../composables/useAppConfirm';
import AppDialog from '../common/AppDialog.vue';
import type { GraphVersionInfo } from '../../features/graph/types';
import { formatTime } from '../../features/graph/utils';

const { t } = useI18n();

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

async function confirmRollback(version: number) {
  const ok = await confirm({
    title: t('graphs.versionPanelConfirmTitle'),
    message: t('graphs.versionPanelConfirmMessage', { version }),
    tone: 'danger',
  });
  if (!ok) return;
  emit('rollback', version);
}
</script>
