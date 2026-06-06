<template>
  <q-card flat class="overview-panel">
    <q-card-section>
      <div class="text-h6 overview-section-title">根因分析</div>
      <div class="text-caption overview-section-caption">失败报告的根因分析与修复建议</div>
    </q-card-section>
    <q-separator class="overview-separator" />
    <q-card-section>
      <div v-if="!cards.length" class="overview-empty">暂无根因分析数据</div>
      <div v-else class="q-gutter-md">
        <q-card v-for="card in cards" :key="card.id" flat bordered class="rca-card">
          <q-card-section class="q-pb-sm">
            <div class="row items-center q-gutter-sm">
              <q-badge rounded color="negative">{{ card.skillId || '未知 Skill' }}</q-badge>
              <span class="text-caption text-grey-7">{{ formatDate(card.createdAt) }}</span>
            </div>
          </q-card-section>
          <q-card-section class="q-pt-none q-pb-sm">
            <div v-if="card.rootCauseAnalysis" class="text-body2 q-mb-sm">
              <span class="text-weight-medium text-grey-8">根因：</span>{{ card.rootCauseAnalysis }}
            </div>
            <div v-if="card.suggestedFix" class="text-body2 q-mb-sm">
              <span class="text-weight-medium text-grey-8">建议修复：</span>{{ card.suggestedFix }}
            </div>
            <div v-if="card.optimizationAdvice" class="text-body2">
              <span class="text-weight-medium text-grey-8">优化建议：</span>{{ card.optimizationAdvice }}
            </div>
          </q-card-section>
          <q-card-section v-if="card.failureTags && card.failureTags.length" class="q-pt-none">
            <q-chip v-for="tag in card.failureTags" :key="tag" dense size="sm" color="negative" text-color="white">
              {{ tag }}
            </q-chip>
          </q-card-section>
        </q-card>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import type { ExperienceReport } from '../../services/kratos/skill_intelligence/v1/index';

defineProps<{
  cards: ExperienceReport[];
}>();

function formatDate(value?: string) {
  if (!value) return '—';
  return new Date(value).toLocaleString('zh-CN', { hour12: false });
}
</script>

<style scoped lang="sass">
.rca-card
  border-radius: 8px
</style>
