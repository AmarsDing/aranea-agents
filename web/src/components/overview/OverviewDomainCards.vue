<template>
  <!-- 基座总览 · 按业务功能分的领域卡：每张回答一个问题，带几个数，点了进这个领域 -->
  <nav class="ov-domains" :aria-label="t('overviewPage.v2.domainsLabel')">
    <router-link v-for="card in cards" :key="card.key" class="ov-domain" :to="card.to">
      <span class="ov-domain__head">
        <span class="ov-domain__icon"><q-icon :name="card.icon" size="18px" aria-hidden="true" /></span>
        <span class="ov-domain__name">{{ card.title }}</span>
      </span>
      <span class="ov-domain__q">{{ card.question }}</span>
      <span class="ov-domain__facts">
        <span v-for="fact in card.facts" :key="fact.label">
          {{ fact.label }}<b>{{ fact.value ?? '—' }}</b>
        </span>
      </span>
    </router-link>
  </nav>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { OvDomainCard } from './ovTypes';

defineProps<{ cards: OvDomainCard[] }>();

const { t } = useI18n();
</script>
