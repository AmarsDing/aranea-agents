<template>
  <AppDialog
    :model-value="open"
    :title="t('knowledgePage.moveConflictTitle')"
    :subtitle="t('knowledgePage.moveConflictMessage', { dir: targetLabel, name: fileName })"
    size="sm"
    @update:model-value="$emit('update:open', $event)"
  >
    <q-list dense>
          <q-item v-ripple clickable :disable="loading" @click="$emit('resolve', 'overwrite')">
            <q-item-section avatar><q-icon name="find_replace" color="warning" /></q-item-section>
            <q-item-section>
              <q-item-label>{{ t('knowledgePage.moveConflictOverwrite') }}</q-item-label>
              <q-item-label caption>{{ t('knowledgePage.moveConflictOverwriteHint') }}</q-item-label>
            </q-item-section>
          </q-item>
          <q-item v-ripple clickable :disable="loading" @click="$emit('resolve', 'rename')">
            <q-item-section avatar><q-icon name="content_copy" color="primary" /></q-item-section>
            <q-item-section>
              <q-item-label>{{ t('knowledgePage.moveConflictRename') }}</q-item-label>
              <q-item-label caption>{{ t('knowledgePage.moveConflictRenameHint') }}</q-item-label>
            </q-item-section>
          </q-item>
        </q-list>
    <q-linear-progress v-if="loading" indeterminate color="primary" class="q-mt-sm" />
    <template #actions>
      <q-btn flat no-caps :disable="loading" :label="t('knowledgePage.moveCancel')" @click="$emit('update:open', false)" />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppDialog from '../../components/common/AppDialog.vue';

// G3-F1 拖拽移动同名冲突弹窗（V12.5）：覆盖（旧版入回收站）/ 保留两份（自动改名）/ 取消。
defineProps<{
  open: boolean;
  /** 冲突文件名。 */
  fileName: string;
  /** 目标目录展示名（末段目录名；根目录由调用方传 i18n 文案）。 */
  targetLabel: string;
  loading: boolean;
}>();

defineEmits<{
  'update:open': [value: boolean];
  resolve: [policy: 'overwrite' | 'rename'];
}>();

const { t } = useI18n();
</script>
