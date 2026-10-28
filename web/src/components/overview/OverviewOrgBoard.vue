<template>
  <q-expansion-item
    dense
    class="ov-org"
    header-class="ov-org__header"
    :label="t('overviewPage.v2.orgTitle')"
    :caption="captionLine"
    expand-icon="unfold_more"
    default-closed
  >
    <div class="ov-org__body">
      <div class="ov-org__disclaimer">
        <q-icon name="info_outline" size="13px" />
        {{ t('overviewPage.v2.orgDisclaimer') }}
      </div>
      <div class="ov-org__headrow">
        <span class="ov-tile__label">{{ t('overviewPage.v2.orgWorstAgents') }}</span>
        <q-btn
          flat
          dense
          no-caps
          size="sm"
          icon-right="open_in_new"
          :label="t('overviewPage.v2.orgViewAll')"
          @click="$emit('drill')"
        />
      </div>
      <div v-if="!usageAvailable || !worstAgents.length" class="ov-trend__empty">
        {{ t(usageAvailable ? 'overviewPage.v2.orgAgentsEmpty' : 'uiSystem.overview.dataUnavailable') }}
      </div>
      <AppRegistryMarkupTable v-else :rows="worstAgents" :columns="columns" row-key="key" table-class="ov-org__table">
        <template #cell-agent="{ row }"
          ><span class="ov-org__name" :title="String(row.key)">{{ row.name }}</span></template
        >
        <template #cell-calls="{ row }">{{ formatCount(Number(row.calls)) }}</template>
        <template #cell-rate="{ row }"
          ><span class="ov-org__rate" :class="`ov-org__rate--${rateTone(Number(row.successRate))}`"
            >{{ (Number(row.successRate) * 100).toFixed(1) }}%</span
          ></template
        >
      </AppRegistryMarkupTable>
    </div>
  </q-expansion-item>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import AppRegistryMarkupTable from '../layout/AppRegistryMarkupTable.vue';
import { buildOverviewAgentColumns } from '../../features/usage/overviewTableUi';
import { useI18n } from 'vue-i18n';
import { formatCount } from '../../features/usage/moneyFormat';
import type { OvWorstAgent } from './ovTypes';

const props = withDefaults(
  defineProps<{
    inventoryAvailable?: boolean;
    usageAvailable?: boolean;
    orgs: number;
    teams: number;
    agentsTotal: number;
    agentsEnabled: number;
    worstAgents: OvWorstAgent[];
  }>(),
  { inventoryAvailable: true, usageAvailable: true },
);

defineEmits<{ drill: [] }>();

const { t } = useI18n();
const columns = computed(() => buildOverviewAgentColumns(t));

const captionLine = computed(() =>
  !props.inventoryAvailable
    ? t('uiSystem.overview.dataUnavailable')
    : t('overviewPage.v2.orgCaption', {
        orgs: props.orgs,
        teams: props.teams,
        enabled: props.agentsEnabled,
        total: props.agentsTotal,
      }),
);

function rateTone(rate: number): 'ok' | 'warn' | 'danger' {
  if (rate >= 0.99) return 'ok';
  if (rate >= 0.95) return 'warn';
  return 'danger';
}
</script>
