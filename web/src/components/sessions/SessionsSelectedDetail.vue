<template>
  <q-card v-if="session" flat class="app-registry-panel q-mb-md">
    <q-card-section class="row items-start justify-between q-gutter-md">
      <div class="col min-width-0">
        <div class="row items-center q-gutter-sm wrap">
          <div class="app-registry-cell-primary text-h6">{{ session.title || "未命名会话" }}</div>
          <q-chip dense :color="ownerChipColor(session.owner_type)" text-color="white">{{ ownerLabel(session.owner_type) }}</q-chip>
          <q-badge :color="statusBadgeColor(session.status)">{{ session.status }}</q-badge>
        </div>
        <div class="app-registry-cell-sub q-mt-xs">
          {{ session.id }} · 创建 {{ formatSessionDate(session.created_at) }} · 最后活跃
          {{ formatSessionDate(session.last_message_at || session.updated_at) }}
        </div>
        <div v-if="session.summary" class="app-registry-cell-desc q-mt-sm">{{ session.summary }}</div>
      </div>
      <div class="row q-gutter-sm shrink-0">
        <q-btn outline rounded no-caps color="primary" icon="chat" label="继续会话" :to="chatRoute" />
        <q-btn flat rounded no-caps icon="archive" label="归档" :disable="session.status === 'archived'" @click="$emit('archive')" />
      </div>
    </q-card-section>
    <q-separator />
    <q-card-section class="row q-col-gutter-md">
      <div class="col-12 col-md-4">
        <div class="app-metrics-card__label">Context</div>
        <q-linear-progress
          rounded
          size="10px"
          :value="ratioValue(session.context_used_ratio)"
          :color="contextProgressColor(session.context_status)"
          class="q-mt-sm"
        />
        <div class="app-registry-cell-sub q-mt-xs">
          当前 {{ formatPercent(session.context_used_ratio) }} · 最高 {{ formatPercent(session.max_context_used_ratio) }}
        </div>
      </div>
      <div class="col-6 col-md-2">
        <div class="app-metrics-card__label">消息</div>
        <div class="app-metrics-card__value">{{ session.message_count }}</div>
      </div>
      <div class="col-6 col-md-2">
        <div class="app-metrics-card__label">模型调用</div>
        <div class="app-metrics-card__value">{{ session.model_call_count }}</div>
      </div>
      <div class="col-6 col-md-2">
        <div class="app-metrics-card__label">Token</div>
        <div class="app-metrics-card__value">{{ formatNumber(session.total_tokens) }}</div>
      </div>
      <div class="col-6 col-md-2">
        <div class="app-metrics-card__label">费用</div>
        <div class="app-metrics-card__value">{{ formatCostMicroUsd(session.total_cost_micro_usd) }}</div>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import type { RouteLocationRaw } from "vue-router";
import type { Session } from "../../features/session/types";
import {
  contextProgressColor,
  formatCostMicroUsd,
  formatNumber,
  formatPercent,
  formatSessionDate,
  ownerChipColor,
  ownerLabel,
  ratioValue,
  statusBadgeColor
} from "./sessionUi";

withDefaults(
  defineProps<{
    session: Session | null;
    chatRoute?: RouteLocationRaw;
  }>(),
  {
    chatRoute: () => ({ name: "chat" })
  }
);

defineEmits<{ archive: [] }>();
</script>
