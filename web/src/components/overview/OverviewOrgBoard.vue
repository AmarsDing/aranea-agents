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
      <div v-if="!worstAgents.length" class="ov-trend__empty">{{ t('overviewPage.v2.orgAgentsEmpty') }}</div>
      <table v-else class="ov-org__table">
        <thead>
          <tr>
            <th class="text-left">{{ t('overviewPage.v2.orgColAgent') }}</th>
            <th class="text-right">{{ t('overviewPage.v2.orgColCalls') }}</th>
            <th class="text-right">{{ t('overviewPage.v2.orgColRate') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="a in worstAgents" :key="a.key">
            <td class="ov-org__name" :title="a.key">{{ a.name }}</td>
            <td class="text-right">{{ formatCount(a.calls) }}</td>
            <td class="text-right ov-org__rate" :class="`ov-org__rate--${rateTone(a.successRate)}`">
              {{ (a.successRate * 100).toFixed(1) }}%
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </q-expansion-item>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { formatCount } from '../../features/usage/moneyFormat';
import type { OvWorstAgent } from './ovTypes';

const props = defineProps<{
  orgs: number;
  teams: number;
  agentsTotal: number;
  agentsEnabled: number;
  worstAgents: OvWorstAgent[];
}>();

defineEmits<{ drill: [] }>();

const { t } = useI18n();

const captionLine = computed(() =>
  t('overviewPage.v2.orgCaption', {
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
