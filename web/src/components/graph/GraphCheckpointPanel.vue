<template>
  <div class="graph-checkpoint-panel">
    <div class="graph-checkpoint-panel__header row items-center q-gutter-sm q-mb-sm">
      <div class="graph-checkpoint-panel__title">检查点</div>
      <q-space />
      <q-btn flat dense round icon="refresh" :loading="loading" @click="$emit('refresh')">
        <q-tooltip>刷新</q-tooltip>
      </q-btn>
    </div>
    <q-spinner v-if="loading" color="primary" size="28px" />
    <div v-else-if="!checkpoints.length" class="text-caption app-text-secondary">暂无检查点记录。</div>
    <q-list v-else dense bordered separator class="rounded-borders">
      <q-item
        v-for="cp in checkpoints"
        :key="cp.checkpointId"
        clickable
        :active="selectedCheckpointId === cp.checkpointId"
        active-class="graph-checkpoint-panel__item--active"
        @click="$emit('select', cp)"
      >
        <q-item-section>
          <q-item-label class="graph-checkpoint-panel__mono">{{ cp.checkpointId }}</q-item-label>
          <q-item-label caption>
            step {{ cp.step }} · {{ cp.source || "checkpoint" }}
          </q-item-label>
          <q-item-label caption>{{ formatTime(cp.timestamp) }}</q-item-label>
        </q-item-section>
        <q-item-section side>
          <q-badge rounded color="blue-grey">{{ cp.namespace || "default" }}</q-badge>
        </q-item-section>
      </q-item>
    </q-list>

    <div v-if="stateSnapshot" class="graph-checkpoint-panel__preview q-mt-md">
      <div class="graph-checkpoint-panel__preview-title row items-center q-gutter-xs">
        <q-icon name="visibility" size="16px" />
        <span>状态预览</span>
      </div>
      <div class="graph-checkpoint-panel__json">{{ snapshotJson }}</div>
      <div v-if="nextNodes.length" class="graph-checkpoint-panel__next q-mt-xs">
        <span class="text-caption">下一步节点：</span>
        <q-badge v-for="n in nextNodes" :key="n" rounded color="primary" :label="n" class="q-mr-xs" />
      </div>
      <q-btn
        dense
        flat
        rounded
        color="accent"
        icon="restore"
        label="回退至此检查点"
        class="q-mt-sm"
        :loading="restoring"
        @click="confirmRestore"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { useQuasar } from "quasar";
import type { CheckpointInfo } from "../../features/graph/types";
import { formatTime } from "../../features/graph/utils";

const props = defineProps<{
  checkpoints: CheckpointInfo[];
  loading: boolean;
  selectedCheckpointId?: string | null;
  stateSnapshot?: Record<string, unknown> | null;
  nextNodes?: string[];
  restoring?: boolean;
}>();

const emit = defineEmits<{
  refresh: [];
  select: [checkpoint: CheckpointInfo];
  restore: [checkpoint: CheckpointInfo];
}>();

const $q = useQuasar();

const snapshotJson = computed(() => {
  if (!props.stateSnapshot) return "";
  try {
    return JSON.stringify(props.stateSnapshot, null, 2);
  } catch {
    return String(props.stateSnapshot);
  }
});

function confirmRestore() {
  const cp = props.checkpoints.find((c) => c.checkpointId === props.selectedCheckpointId);
  if (!cp) return;
  $q.dialog({
    title: "确认回退",
    message: `将回退至检查点 ${cp.checkpointId}（step ${cp.step}），当前执行状态将被替换。确定继续？`,
    cancel: true,
    persistent: true,
  }).onOk(() => {
    emit("restore", cp);
  });
}
</script>
