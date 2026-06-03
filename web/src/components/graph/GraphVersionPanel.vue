<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="graph-version-panel app-dialog-card app-dialog-card--sm app-glass-dialog">
      <q-card-section class="app-glass-dialog__head">
        <div class="app-glass-dialog__title">版本历史</div>
        <q-space />
        <q-btn flat dense round icon="close" @click="$emit('update:modelValue', false)" />
      </q-card-section>
      <q-separator />
      <q-card-section class="app-dialog-body app-glass-dialog__body">
        <div v-if="loading" class="row justify-center q-py-lg">
          <q-spinner color="primary" size="28px" />
        </div>
        <div v-else-if="!versions.length" class="text-caption text-grey-7 text-center q-py-lg">
          暂无历史版本（保存后会自动创建快照）
        </div>
        <q-list v-else bordered separator class="rounded-borders">
          <q-item v-for="item in versions" :key="item.version">
            <q-item-section>
              <q-item-label>v{{ item.version }} · {{ item.name || '未命名' }}</q-item-label>
              <q-item-label caption>{{ formatTime(item.savedAt) }}</q-item-label>
            </q-item-section>
            <q-item-section side>
              <q-btn
                flat
                dense
                color="primary"
                label="回滚"
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
import { useQuasar } from 'quasar';
import type { GraphVersionInfo } from '../../features/graph/types';
import { formatTime } from '../../features/graph/utils';

const $q = useQuasar();

const props = defineProps<{
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
    title: '回滚版本',
    message: `确定回滚到 v${version}？当前版本将被覆盖。`,
    cancel: true,
    persistent: true,
  }).onOk(() => {
    emit('rollback', version);
  });
}
</script>
