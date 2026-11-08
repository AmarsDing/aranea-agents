<script setup lang="ts">
// 老板视图 / 值班视图切换（整合 P2-8）：原 App.vue 的内存态 viewMode 在
// aranea 落地为两条路由（/ceo/overview、/ceo/oncall），同一口径两个入口。
import { useI18n } from 'vue-i18n';
import { useRoute, useRouter } from 'vue-router';

const { t } = useI18n();
const route = useRoute();
const router = useRouter();

function go(name: 'ceo-overview' | 'ceo-oncall') {
  if (route.name !== name) void router.push({ name });
}
</script>

<template>
  <div class="ceo-viewswitch" role="tablist">
    <button
      role="tab"
      :aria-selected="route.name === 'ceo-overview'"
      :class="{ on: route.name === 'ceo-overview' }"
      @click="go('ceo-overview')"
    >
      {{ t('ceoboard.page.viewBoss') }}
    </button>
    <button
      role="tab"
      :aria-selected="route.name === 'ceo-oncall'"
      :class="{ on: route.name === 'ceo-oncall' }"
      @click="go('ceo-oncall')"
    >
      {{ t('ceoboard.page.viewOncall') }}
    </button>
  </div>
</template>
