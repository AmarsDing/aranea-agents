<template>
  <q-card v-if="session" flat class="sessions-detail-card q-mb-md">
    <q-card-section class="row items-start justify-between q-gutter-md">
      <div class="col">
        <div class="row items-center q-gutter-sm">
          <div class="text-h6" style="color: var(--color-text-primary)">{{ session.title }}</div>
          <q-chip dense :color="ownerChipColor(session.owner_type)" text-color="white">{{ ownerLabel(session.owner_type) }}</q-chip>
          <q-badge :color="statusBadgeColor(session.status)">{{ session.status }}</q-badge>
        </div>
        <div class="sessions-muted text-caption q-mt-xs">
          {{ session.id }} · 创建 {{ formatSessionDate(session.created_at) }} · 最后活跃
          {{ formatSessionDate(session.last_message_at || session.updated_at) }}
        </div>
        <div v-if="session.summary" class="text-body2 q-mt-sm" style="color: var(--color-text-primary)">{{ session.summary }}</div>
      </div>
      <div class="row q-gutter-sm">
        <q-btn outline rounded icon="chat" label="继续会话" class="sessions-btn-accent-outline" :to="chatRoute" />
        <q-btn flat rounded icon="archive" label="归档" class="sessions-btn-ghost" :disable="session.status === 'archived'" @click="$emit('archive')" />
      </div>
    </q-card-section>
    <q-separator class="sessions-sep" />
    <q-card-section class="row q-col-gutter-md">
      <div class="col-12 col-md-4">
        <div class="sessions-muted text-caption">Context</div>
        <q-linear-progress
          rounded
          size="12px"
          :value="ratioValue(session.context_used_ratio)"
          :color="contextProgressColor(session.context_status)"
          class="q-mt-sm"
        />
        <div class="sessions-muted text-caption q-mt-xs">
          当前 {{ formatPercent(session.context_used_ratio) }} · 最高 {{ formatPercent(session.max_context_used_ratio) }}
        </div>
      </div>
      <div class="col-6 col-md-2">
        <div class="sessions-muted text-caption">消息</div>
        <div class="text-h6" style="color: var(--color-text-primary)">{{ session.message_count }}</div>
      </div>
      <div class="col-6 col-md-2">
        <div class="sessions-muted text-caption">模型调用</div>
        <div class="text-h6" style="color: var(--color-text-primary)">{{ session.model_call_count }}</div>
      </div>
      <div class="col-6 col-md-2">
        <div class="sessions-muted text-caption">Token</div>
        <div class="text-h6" style="color: var(--color-text-primary)">{{ formatNumber(session.total_tokens) }}</div>
      </div>
      <div class="col-6 col-md-2">
        <div class="sessions-muted text-caption">费用</div>
        <div class="text-h6" style="color: var(--color-text-primary)">{{ formatCostMicroUsd(session.total_cost_micro_usd) }}</div>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import type { RouteLocationRaw } from "vue-router";
import type { Session } from "../../features/chat/api";
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
