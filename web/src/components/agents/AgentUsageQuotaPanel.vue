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

    <q-card flat bordered>
      <q-card-section>
        <div class="row q-col-gutter-md">
          <q-input
            v-model.number="monthlyUsd"
            class="col-12 col-md-4"
            dense
            outlined
            type="number"
            min="0"
            step="0.01"
            prefix="$"
            label="月预算 (USD)"
            hint="0 表示不限制"
          />
          <q-input v-model="periodStart" class="col-12 col-md-4" dense outlined label="周期开始" placeholder="YYYY-MM-DD" />
          <q-input v-model="periodEnd" class="col-12 col-md-4" dense outlined label="周期结束" placeholder="YYYY-MM-DD" />
        </div>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat rounded label="检查用量" :loading="checking" :disable="!agentId" @click="runCheck" />
        <q-btn color="primary" rounded unelevated label="保存配额" :loading="saving" :disable="!agentId" @click="saveQuota" />
      </q-card-actions>
    </q-card>

    <q-card flat bordered class="q-mt-md">
      <q-card-section>
        <div class="text-subtitle2 q-mb-sm">预算告警阈值</div>
        <div class="text-caption text-grey-7 q-mb-md">
          当月消耗达到月预算比例时写入监控事件（`usage.budget_alert`），同一阈值 60 分钟内不重复通知。
        </div>
        <div class="row q-col-gutter-md items-end">
          <q-input
            v-model.number="alertRatioPct"
            class="col-12 col-md-4"
            dense
            outlined
            type="number"
            min="1"
            max="100"
            suffix="%"
            label="告警比例"
          />
          <q-toggle v-model="alertEnabled" label="启用" />
          <q-btn color="primary" rounded unelevated label="保存告警" :loading="alertSaving" :disable="!agentId" @click="saveAlert" />
        </div>
      </q-card-section>
    </q-card>

    <q-card v-if="check && agentId" flat bordered class="q-mt-md">
      <q-card-section>
        <div class="text-subtitle2 q-mb-sm">当前周期</div>
        <q-chip
          :color="check.allowed ? 'positive' : 'negative'"
          text-color="white"
          :label="check.allowed ? '允许继续对话' : '已超限'"
        />
        <div class="text-body2 q-mt-sm text-grey-8">{{ check.reason }}</div>
        <div class="row q-col-gutter-md q-mt-md">
          <div class="col-12 col-md-4">
            <div class="text-caption text-grey-7">已消耗</div>
            <div class="text-h6">${{ microUsdToUsd(check.spent_micro_usd) }}</div>
          </div>
          <div class="col-12 col-md-4">
            <div class="text-caption text-grey-7">剩余</div>
            <div class="text-h6">${{ microUsdToUsd(check.remaining_micro_usd) }}</div>
          </div>
          <div v-if="check.quota?.monthly_micro_usd" class="col-12 col-md-4">
            <div class="text-caption text-grey-7">月上限</div>
            <div class="text-h6">${{ microUsdToUsd(check.quota.monthly_micro_usd) }}</div>
          </div>
        </div>
      </q-card-section>
    </q-card>
  </section>
</template>

<script setup lang="ts">
import { toRef } from "vue";
import { useAgentUsageQuota } from "../../features/usage/useAgentUsageQuota";

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
  alertRatioPct,
  alertEnabled,
  alertSaving,
  microUsdToUsd,
  loadQuota,
  runCheck,
  saveQuota,
  saveAlert
} = useAgentUsageQuota(toRef(props, "agentId"));
</script>
