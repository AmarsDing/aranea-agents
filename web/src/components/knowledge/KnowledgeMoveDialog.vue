<template>
  <q-dialog :model-value="open" @update:model-value="$emit('update:open', !!$event)">
    <q-card class="app-dialog-card app-dialog-card--sm">
      <q-card-section>
        <div class="text-h6">{{ t('knowledgePage.moveDialogTitle') }}</div>
        <div class="text-caption text-grey-7 ellipsis">{{ docSource }}</div>
      </q-card-section>
      <q-card-section class="q-pt-none">
        <q-select
          :model-value="targetId"
          dense
          outlined
          emit-value
          map-options
          options-dense
          :options="options"
          :label="t('knowledgePage.moveTargetLabel')"
          @update:model-value="$emit('update:targetId', String($event ?? ''))"
        >
          <template #option="scope">
            <q-item v-bind="scope.itemProps" :disable="scope.opt.disable">
              <q-item-section>
                <q-item-label>{{ scope.opt.label }}</q-item-label>
                <q-item-label v-if="scope.opt.disable" caption class="text-warning">
                  {{ t('knowledgePage.moveDimMismatch', { dim: scope.opt.dim }) }}
                </q-item-label>
              </q-item-section>
            </q-item>
          </template>
          <template #no-option>
            <q-item>
              <q-item-section class="text-grey-7">{{ t('knowledgePage.moveEmpty') }}</q-item-section>
            </q-item>
          </template>
        </q-select>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn v-close-popup flat no-caps :label="t('knowledgePage.moveCancel')" />
        <q-btn
          color="primary"
          unelevated
          no-caps
          :label="t('knowledgePage.moveSubmit')"
          :disable="!targetId"
          :loading="loading"
          @click="$emit('submit')"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';

// US-14 文档跨库移动对话框：选项中 dim 与源库不一致的目标库被禁用并提示原因。
defineProps<{
  open: boolean;
  targetId: string;
  docSource: string;
  options: Array<{ label: string; value: string; disable?: boolean; dim?: number }>;
  loading: boolean;
}>();

defineEmits<{
  'update:open': [value: boolean];
  'update:targetId': [value: string];
  submit: [];
}>();

const { t } = useI18n();
</script>
