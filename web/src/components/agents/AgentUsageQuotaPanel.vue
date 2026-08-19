<template>
  <section class="settings-section">
    <div class="section-heading">
      <div>
        <div class="text-subtitle1 text-weight-bold">用量配额</div>
        <div class="text-caption text-grey-7">
          按本 Agent 配置月度费用上限（USD）；Chat 每次对话前自动校验，超限将拒绝新 Turn。
        </div>
      </div>
      <q-btn outline rounded dense color="primary" icon="refresh" label="刷新" :loading="checking" @click="loadQuota" />
    </div>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">{{ error }}</q-banner>

    <q-card flat bordered class="capability-card">
      <q-card-section>
        <div class="app-form-field-grid">
          <q-input
            v-model.number="monthlyUsd"
            dense
            outlined
            type="number"
            min="0"
            step="0.01"
            prefix="$"
            label="月预算 (USD)"
            hint="留空或 0 表示不限制"
            lazy-rules
            :rules="monthlyUsdRules"
          />
          <q-input
            v-model="periodStart"
            dense
            outlined
            label="周期开始"
            placeholder="YYYY-MM-DD"
            hint="留空自动取当自然月；过期后自动重置"
            lazy-rules
            :rules="periodStartRules"
          />
          <q-input
            v-model="periodEnd"
            dense
            outlined
            label="周期结束"
            placeholder="YYYY-MM-DD"
            hint="留空自动取当自然月；过期后自动重置"
            lazy-rules
            :rules="periodEndRules"
          />
        </div>
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn
          color="primary"
          rounded
          unelevated
          no-caps
          label="保存配额"
          :loading="saving"
          :disable="!agentId"
          @click="saveQuota"
        />
      </q-card-actions>
    </q-card>

    <q-card flat bordered class="capability-card q-mt-md">
      <q-card-section>
        <div class="text-subtitle2 q-mb-sm">预算告警阈值</div>
        <div class="text-caption text-grey-7 q-mb-md">
          当月消耗达到月预算比例时写入监控事件（<code>usage.budget_alert</code>），同一阈值 60 分钟内不重复通知。
        </div>
        <div class="app-form-field-grid app-form-field-grid--2col items-end">
          <q-input
            v-model.number="alertRatioPct"
            dense
            outlined
            type="number"
            min="1"
            max="100"
            suffix="%"
            label="告警比例"
            lazy-rules
            :rules="alertRatioRules"
          />
          <q-toggle v-model="alertEnabled" label="启用" />
        </div>
        <div class="app-actions-bar app-actions-bar--start q-mt-md">
          <q-btn
            color="primary"
            rounded
            unelevated
            no-caps
            label="保存告警"
            :loading="alertSaving"
            :disable="!agentId"
            @click="saveAlert"
          />
        </div>
      </q-card-section>
    </q-card>

    <q-card v-if="check && agentId" flat bordered class="capability-card q-mt-md">
      <q-card-section>
        <div class="text-subtitle2 q-mb-sm">当前周期</div>
        <q-chip
          :color="check.allowed ? 'positive' : 'negative'"
          text-color="white"
          :label="check.allowed ? '允许继续对话' : '已超限'"
        />
        <div class="text-body2 q-mt-sm text-grey-8">{{ reasonText }}</div>
        <div v-if="hasActiveQuota" class="app-form-field-grid q-mt-md">
          <div>
            <div class="text-caption text-grey-7">已消耗</div>
            <div class="text-h6">${{ microUsdToUsd(check.spent_micro_usd) }}</div>
          </div>
          <div>
            <div class="text-caption text-grey-7">{{ $t('usageQuota.remaining') }}</div>
            <div class="text-h6">${{ microUsdToUsd(check.remaining_micro_usd) }}</div>
          </div>
          <div v-if="check.quota?.monthly_micro_usd">
            <div class="text-caption text-grey-7">{{ $t('usageQuota.monthlyLimit') }}</div>
            <div class="text-h6">${{ microUsdToUsd(check.quota.monthly_micro_usd) }}</div>
          </div>
        </div>
      </q-card-section>
    </q-card>
  </section>
</template>

<script setup lang="ts">
import { toRef } from 'vue';
import { useAgentUsageQuota } from '../../features/usage/useAgentUsageQuota';

const props = defineProps<{
  agentId: string;
}>();

const {
  monthlyUsd,
  periodStart,
  periodEnd,
  saving,
  checking,
  error,
  check,
  reasonText,
  hasActiveQuota,
  alertRatioPct,
  alertEnabled,
  alertSaving,
  monthlyUsdRules,
  periodStartRules,
  periodEndRules,
  alertRatioRules,
  microUsdToUsd,
  loadQuota,
  saveQuota,
  saveAlert,
} = useAgentUsageQuota(toRef(props, 'agentId'));
</script>
