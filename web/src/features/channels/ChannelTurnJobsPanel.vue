// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <div class="channel-turn-jobs-panel">
    <div class="row items-center justify-between q-mb-sm">
      <div class="text-subtitle2">{{ t("channelEditor.turnJobsTitle") }}</div>
      <q-btn
        flat
        dense
        no-caps
        icon="refresh"
        :label="t('channelEditor.turnJobsRefresh')"
        :loading="loading"
        @click="load"
      />
    </div>
    <p class="text-caption text-grey-7 q-mb-sm">{{ t("channelEditor.turnJobsHint") }}</p>
    <q-banner v-if="error" dense rounded class="bg-negative text-white q-mb-sm">{{ error }}</q-banner>
    <q-table
      flat
      bordered
      dense
      :rows="rows"
      :columns="columns"
      row-key="id"
      :loading="loading"
      hide-pagination
      :pagination="{ rowsPerPage: 10 }"
      class="channel-turn-jobs-table"
    >
      <template #body-cell-status="props">
        <q-td :props="props">
          <q-badge :color="statusColor(String(props.row.status))" :label="props.row.status" />
        </q-td>
      </template>
      <template #no-data>
        <div class="full-width text-center text-grey-6 q-pa-md">{{ t("channelEditor.turnJobsEmpty") }}</div>
      </template>
    </q-table>
  </div>
</template>

<script setup lang="ts">
import { useChannelTurnJobsPanel } from "./useChannelTurnJobsPanel";

const props = defineProps<{ channelId: string }>();

const { t, loading, error, rows, columns, statusColor, load } = useChannelTurnJobsPanel(() => props.channelId);
</script>

<style scoped>
.channel-turn-jobs-table :deep(.q-table__middle) {
  max-height: 240px;
}
</style>
