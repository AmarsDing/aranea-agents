<template>
  <q-dialog :model-value="open" @update:model-value="$emit('update:open', $event)">
    <q-card class="plugin-detail-card app-dialog-card app-dialog-card--md app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-start justify-between q-gutter-md">
        <div class="min-width-0">
          <div class="app-glass-dialog__title">{{ target?.name }}</div>
          <div class="app-glass-dialog__subtitle">{{ target?.key }}</div>
        </div>
        <q-btn v-close-popup flat dense round icon="close" :aria-label="t('pluginsPage.detail.close')" />
      </q-card-section>
      <q-separator />
      <q-card-section v-if="target" class="app-dialog-body q-gutter-md">
        <p class="plugin-detail-desc">{{ target.description || t('pluginsPage.noDescription') }}</p>

        <div class="plugin-detail-metrics row q-col-gutter-sm">
          <div v-for="metric in metrics" :key="metric.label" class="col-6 col-sm-4">
            <div class="plugin-detail-metric">
              <span class="plugin-detail-metric__label">{{ metric.label }}</span>
              <span v-if="metric.tagClass" class="plugin-tag" :class="metric.tagClass">{{ metric.value }}</span>
              <span v-else class="plugin-detail-metric__value">{{ metric.value }}</span>
            </div>
          </div>
        </div>

        <div v-if="target.permissions?.can_edit_config" class="row q-gutter-sm">
          <q-btn
            outline
            dense
            no-caps
            icon="arrow_upward"
            :label="t('pluginsPage.detail.moveUp')"
            :loading="bumpingSort"
            @click="$emit('bumpSort', -10)"
          />
          <q-btn
            outline
            dense
            no-caps
            icon="arrow_downward"
            :label="t('pluginsPage.detail.moveDown')"
            :loading="bumpingSort"
            @click="$emit('bumpSort', 10)"
          />
        </div>

        <q-expansion-item dense-toggle :label="t('pluginsPage.detail.agentBinding')">
          <div class="q-gutter-sm">
            <q-radio
              :model-value="scopeMode"
              val="global"
              :label="t('pluginsPage.detail.scopeGlobal')"
              :disable="!target.permissions?.can_edit_config"
              @update:model-value="$emit('update:scopeMode', $event as 'global' | 'agent')"
            />
            <q-radio
              :model-value="scopeMode"
              val="agent"
              :label="t('pluginsPage.detail.scopeAgent')"
              :disable="!target.permissions?.can_edit_config"
              @update:model-value="$emit('update:scopeMode', $event as 'global' | 'agent')"
            />
            <q-select
              v-if="scopeMode === 'agent'"
              :model-value="scopeAgentId"
              :options="filteredAgentOptions"
              dense
              outlined
              emit-value
              map-options
              use-input
              hide-selected
              fill-input
              input-debounce="0"
              :label="t('pluginsPage.detail.selectAgent')"
              :disable="!target.permissions?.can_edit_config"
              @filter="filterAgentOptions"
              @update:model-value="$emit('update:scopeAgentId', String($event ?? ''))"
            />
            <q-btn
              color="primary"
              rounded
              unelevated
              no-caps
              :label="t('pluginsPage.detail.saveScope')"
              :loading="savingScope"
              :disable="!target.permissions?.can_edit_config"
              @click="$emit('saveScope')"
            />
          </div>
        </q-expansion-item>

        <q-expansion-item dense-toggle default-opened :label="t('pluginsPage.detail.callbacks')">
          <div class="app-registry-chip-wrap">
            <span v-for="point in target.callback_points" :key="point" class="plugin-tag plugin-tag--callback">{{
              point
            }}</span>
            <span v-if="!target.callback_points?.length" class="text-grey-7">{{ t('pluginsPage.noCallbacks') }}</span>
          </div>
        </q-expansion-item>

        <q-expansion-item dense-toggle :label="t('pluginsPage.detail.configJson')">
          <pre class="app-code-block app-code-block--compact">{{
            prettyJSON(target.config_json, t('pluginsPage.detail.noConfig'))
          }}</pre>
        </q-expansion-item>
        <q-expansion-item dense-toggle :label="t('pluginsPage.detail.defaultConfig')">
          <pre class="app-code-block app-code-block--compact">{{
            prettyJSON(target.default_config_json, t('pluginsPage.detail.noDefaultConfig'))
          }}</pre>
        </q-expansion-item>
        <q-expansion-item dense-toggle :label="t('pluginsPage.detail.configSchema')">
          <pre class="app-code-block app-code-block--compact">{{
            prettyJSON(target.config_schema_json, t('pluginsPage.detail.noSchema'))
          }}</pre>
        </q-expansion-item>
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Plugin } from '../../features/plugins/types';
import { formatDate } from '../../shared/format';
import { lastStatusLabel, lastStatusTagClass, prettyJSON, riskTagClass } from './pluginUi';

const props = defineProps<{
  open: boolean;
  target: Plugin | null;
  scopeMode: 'global' | 'agent';
  scopeAgentId: string;
  agentOptions: { label: string; value: string }[];
  savingScope: boolean;
  bumpingSort: boolean;
}>();

defineEmits<{
  'update:open': [value: boolean];
  'update:scopeMode': [value: 'global' | 'agent'];
  'update:scopeAgentId': [value: string];
  bumpSort: [delta: number];
  saveScope: [];
}>();

const { t } = useI18n();

// 指定 Agent 下拉：数量多（20+）时支持输入过滤（label/value 双向匹配）
const filteredAgentOptions = ref(props.agentOptions);

function filterAgentOptions(val: string, update: (fn: () => void) => void) {
  update(() => {
    const needle = val.toLowerCase();
    filteredAgentOptions.value = props.agentOptions.filter(
      (o) => o.label.toLowerCase().includes(needle) || o.value.toLowerCase().includes(needle),
    );
  });
}

watch(
  () => props.agentOptions,
  (opts) => {
    filteredAgentOptions.value = opts;
  },
);

const metrics = computed(() => {
  const target = props.target;
  if (!target) return [];
  return [
    { label: t('pluginsPage.detail.metricCategory'), value: target.category },
    {
      label: t('pluginsPage.detail.metricRisk'),
      value: target.risk_level,
      tagClass: riskTagClass(target.risk_level),
    },
    { label: t('pluginsPage.detail.metricScope'), value: target.scope || 'global' },
    { label: t('pluginsPage.detail.metricSort'), value: String(target.sort_order) },
    { label: t('pluginsPage.detail.metricInvokeCount'), value: String(target.invoke_count) },
    {
      label: t('pluginsPage.detail.metricBlockError'),
      value: `${target.block_count} / ${target.error_count}`,
    },
    {
      label: t('pluginsPage.detail.metricLastStatus'),
      value: lastStatusLabel(target, t),
      tagClass: lastStatusTagClass(target),
    },
    { label: t('pluginsPage.detail.metricLastInvoke'), value: formatDate(target.last_invoked_at) },
  ];
});
</script>
