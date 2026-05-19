<template>
  <q-page class="app-page-cream q-pa-sm q-pa-md-md">
    <section class="row items-center justify-between q-mb-md">
      <div>
        <div class="text-h5">用量配额</div>
        <div class="text-caption text-grey-7">按 Agent 配置月度费用上限（USD），Chat turn 前自动校验。</div>
      </div>
      <q-btn outline rounded color="primary" icon="refresh" label="刷新" :loading="loadingAgents" @click="loadAgents" />
    </section>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">{{ error }}</q-banner>

    <q-card flat bordered class="q-mb-md">
      <q-card-section>
        <div class="row q-col-gutter-md">
          <q-select
            :model-value="scopeId"
            class="col-12 col-md-6"
            dense
            outlined
            emit-value
            map-options
            label="Agent"
            :options="agentOptions"
            :loading="loadingAgents"
            @update:model-value="onAgentChange"
          />
          <q-input
            v-model.number="monthlyUsd"
            class="col-12 col-md-3"
            dense
            outlined
            type="number"
            min="0"
            step="0.01"
            prefix="$"
            label="月预算 (USD)"
            hint="0 表示不限制"
          />
          <q-input v-model="periodStart" class="col-12 col-md-3" dense outlined label="周期开始" placeholder="YYYY-MM-DD" />
          <q-input v-model="periodEnd" class="col-12 col-md-3" dense outlined label="周期结束" placeholder="YYYY-MM-DD" />
        </div>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat rounded label="检查用量" :loading="checking" :disable="!scopeId" @click="runCheck" />
        <q-btn color="primary" rounded unelevated label="保存配额" :loading="saving" :disable="!scopeId" @click="saveQuota" />
      </q-card-actions>
    </q-card>

    <q-card v-if="check && scopeId" flat bordered>
      <q-card-section>
        <div class="text-subtitle1 q-mb-sm">配额检查</div>
        <q-chip :color="check.allowed ? 'positive' : 'negative'" text-color="white" :label="check.allowed ? '允许继续对话' : '已超限'" />
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
  </q-page>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useUsageQuotasPage } from "../features/usage/useUsageQuotasPage";

const {
  agents,
  loadingAgents,
  scopeId,
  monthlyUsd,
  periodStart,
  periodEnd,
  saving,
  checking,
  error,
  check,
  microUsdToUsd,
  loadAgents,
  runCheck,
  saveQuota,
  onAgentChange
} = useUsageQuotasPage();

const agentOptions = computed(() =>
  agents.value.map((a) => ({
    label: a.display_name || a.agent_key || a.id,
    value: a.id
  }))
);
</script>
