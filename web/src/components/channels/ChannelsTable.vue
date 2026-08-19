<template>
  <AppRegistryTable
    table-class="channels-data-table"
    :rows="rows"
    :columns="CHANNEL_TABLE_COLUMNS"
    row-key="id"
    :loading="loading"
    hide-pagination
    :pagination="{ rowsPerPage: 0 }"
  >
    <template #body-cell-name="props">
      <q-td :props="props">
        <div class="row items-center no-wrap q-gutter-sm">
          <channel-platform-avatar
            :type="channelType(props.row)"
            :label="props.row.name"
            :metadata="channelMetadata(props.row)"
            size="32px"
          />
          <div class="min-width-0">
            <div class="row items-center no-wrap q-gutter-xs">
              <q-icon v-if="isChannelConnected(props.row)" name="circle" color="positive" size="10px">
                <q-tooltip>{{ t('channelsPage.statusConnected') }}</q-tooltip>
              </q-icon>
              <div class="app-registry-cell-primary ellipsis">{{ props.row.name }}</div>
              <q-badge v-if="isChannelShared(props.row)" outline color="grey-7" class="channel-shared-badge">
                {{ t('channelsPage.sharedBadge') }}
                <q-tooltip>{{ t('channelsPage.sharedReadonly') }}</q-tooltip>
              </q-badge>
            </div>
            <div class="app-registry-cell-sub ellipsis">{{ props.row.key }}</div>
          </div>
        </div>
      </q-td>
    </template>

    <template #body-cell-type="props">
      <q-td :props="props">
        <span class="channel-tag channel-tag--type">{{ catalogLabelForType(catalog, channelType(props.row)) }}</span>
        <span class="channel-tag channel-tag--mode">{{ receiveMode(props.row) }}</span>
      </q-td>
    </template>

    <template #body-cell-status="props">
      <q-td :props="props">
        <AppRegistryHoverTip :text="channelMetadata(props.row).last_error_message">
          <div class="row items-center no-wrap q-gutter-xs">
            <q-icon v-if="isChannelConnected(props.row)" name="circle" color="positive" size="10px" />
            <q-badge rounded :color="channelStatusBadgeColor(props.row)">
              {{ channelStatusBadgeText(props.row) }}
            </q-badge>
          </div>
        </AppRegistryHoverTip>
      </q-td>
    </template>

    <template #body-cell-enabled="props">
      <q-td :props="props">
        <q-toggle
          :model-value="props.row.enabled"
          class="channel-enable-toggle"
          :disable="togglingId === props.row.id || isChannelShared(props.row)"
          @update:model-value="$emit('toggleEnabled', props.row, Boolean($event))"
        >
          <q-tooltip v-if="isChannelShared(props.row)">{{ t('channelsPage.sharedReadonly') }}</q-tooltip>
        </q-toggle>
      </q-td>
    </template>

    <template #body-cell-updated="props">
      <q-td :props="props">{{ formatChannelDate(props.row.updated_at) }}</q-td>
    </template>

    <template #body-cell-external_id="props">
      <q-td :props="props">
        <span class="app-registry-cell-sub ellipsis" :title="channelExternalID(props.row)">{{
          channelExternalID(props.row)
        }}</span>
      </q-td>
    </template>

    <template #body-cell-actions="props">
      <q-td :props="props">
        <div class="app-registry-cell-actions">
          <q-btn
            v-if="channelSupportsWebhook(props.row, catalog)"
            flat
            dense
            round
            class="channel-icon-btn"
            icon="link"
            @click="$emit('copyWebhook', props.row)"
          >
            <q-tooltip>{{ t('channelsPage.copyWebhook') }}</q-tooltip>
          </q-btn>
          <q-btn flat dense round class="channel-icon-btn" icon="work_history" @click="$emit('ops', props.row)">
            <q-tooltip>{{ t('channelsPage.viewOps') }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            class="channel-icon-btn"
            icon="science"
            :loading="testingId === props.row.id"
            :disable="isChannelShared(props.row)"
            @click="$emit('testConnection', props.row)"
          >
            <q-tooltip>{{
              isChannelShared(props.row) ? t('channelsPage.sharedReadonly') : t('channelsPage.testConnection')
            }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            class="channel-icon-btn"
            icon="edit"
            :disable="isChannelShared(props.row)"
            @click="$emit('edit', props.row)"
          >
            <q-tooltip>{{
              isChannelShared(props.row) ? t('channelsPage.sharedReadonly') : t('channelsPage.edit')
            }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            class="channel-icon-btn channel-icon-btn--danger"
            icon="delete"
            :disable="isChannelShared(props.row)"
            @click="$emit('remove', props.row)"
          >
            <q-tooltip>{{
              isChannelShared(props.row) ? t('channelsPage.sharedReadonly') : t('channelsPage.delete')
            }}</q-tooltip>
          </q-btn>
        </div>
      </q-td>
    </template>
  </AppRegistryTable>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../layout/AppRegistryHoverTip.vue';
import ChannelPlatformAvatar from './ChannelPlatformAvatar.vue';
import type { ChannelTypeItem, ChannelRow } from '../../features/channels/types';

import {
  channelTableColumns,
  catalogLabelForType,
  channelExternalID,
  channelMetadata,
  channelStatusBadgeColor,
  channelStatusBadgeText,
  channelSupportsWebhook,
  channelType,
  formatChannelDate,
  isChannelConnected,
  isChannelShared,
  receiveMode,
} from './channelUi';

const { t } = useI18n();

const CHANNEL_TABLE_COLUMNS = channelTableColumns(t);

defineProps<{
  rows: ChannelRow[];
  catalog: ChannelTypeItem[];
  loading: boolean;
  togglingId: string;
  testingId: string;
}>();

defineEmits<{
  toggleEnabled: [row: ChannelRow, value: boolean];
  testConnection: [row: ChannelRow];
  copyWebhook: [row: ChannelRow];
  ops: [row: ChannelRow];
  edit: [row: ChannelRow];
  remove: [row: ChannelRow];
}>();
</script>
