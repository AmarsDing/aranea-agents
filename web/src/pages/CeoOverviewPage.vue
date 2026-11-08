<script setup lang="ts">
// /ceo/overview 老板视图（整合 P2-8）：结论先行 —— 总览 → 需要我处理 → 业务线卡片。
// 与值班视图共用同一份物化数据口径（同一 GetOverview RPC），仅布局不同。
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useAuthStore } from '../stores/auth';
import { useCeoboard } from '../features/ceoboard/useCeoboard';
import CeoViewSwitch from '../components/ceoboard/CeoViewSwitch.vue';
import CeoPlatformChips from '../components/ceoboard/CeoPlatformChips.vue';
import CeoHeroBanner from '../components/ceoboard/CeoHeroBanner.vue';
import CeoDecisionPanel from '../components/ceoboard/CeoDecisionPanel.vue';
import CeoBizLaneCard from '../components/ceoboard/CeoBizLaneCard.vue';
import CeoNodeDrawer from '../components/ceoboard/CeoNodeDrawer.vue';

const { t } = useI18n();
const auth = useAuthStore();
const { ov, err, drawer, openNode, closeDrawer } = useCeoboard();

const meName = computed(() => auth.user?.name?.trim() || auth.displayLabel || undefined);
const updatedAt = computed(() => (ov.value ? new Date(ov.value.as_of).toLocaleString() : ''));
</script>

<template>
  <q-page class="ceo-page">
    <div class="ceo-page-bar">
      <CeoViewSwitch />
      <CeoPlatformChips v-if="ov" :platforms="ov.platforms" />
    </div>

    <div v-if="err" class="ceo-alerts" role="alert">
      <div class="ceo-alert red">
        <span class="lvl">{{ t('ceoboard.page.bannerBoard') }}</span><span>{{ err }}</span>
      </div>
    </div>

    <template v-if="ov">
      <CeoHeroBanner :ov="ov" :me-name="meName" />
      <CeoDecisionPanel :ov="ov" />
      <div class="ceo-biz-grid">
        <CeoBizLaneCard v-for="lane in ov.lanes" :key="lane.biz_line" :lane="lane" @open-node="openNode" />
      </div>
      <div class="ceo-footer">
        {{ t('ceoboard.page.updatedAt', { at: updatedAt }) }} · {{ t('ceoboard.page.autoRefresh') }}
      </div>
    </template>

    <CeoNodeDrawer v-if="drawer" :detail="drawer" @close="closeDrawer" />
  </q-page>
</template>
