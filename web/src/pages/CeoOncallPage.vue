<script setup lang="ts">
// /ceo/oncall 值班视图（整合 P2-8）：工程细节 —— 注册表/权限横幅 + 原始告警 +
// 待拍板 + 泳道节点全表。与老板视图共用同一份物化数据（同一 GetOverview RPC）。
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useAuthStore } from '../stores/auth';
import { useNavStore } from '../stores/nav';
import { useCeoboard } from '../features/ceoboard/useCeoboard';
import CeoViewSwitch from '../components/ceoboard/CeoViewSwitch.vue';
import CeoPlatformChips from '../components/ceoboard/CeoPlatformChips.vue';
import CeoAlertList from '../components/ceoboard/CeoAlertList.vue';
import CeoApprovalsPanel from '../components/ceoboard/CeoApprovalsPanel.vue';
import CeoLaneGroup from '../components/ceoboard/CeoLaneGroup.vue';
import CeoNodeDrawer from '../components/ceoboard/CeoNodeDrawer.vue';

const { t } = useI18n();
const auth = useAuthStore();
const nav = useNavStore();
const { ov, err, drawer, openNode, closeDrawer } = useCeoboard();

const updatedAt = computed(() => (ov.value ? new Date(ov.value.as_of).toLocaleString() : ''));
const meName = computed(() => auth.user?.name?.trim() || auth.displayLabel);
const meRoles = computed(() => nav.roles.join('/'));
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
      <div v-if="ov.registry_notice" class="ceo-alerts">
        <div class="ceo-alert yellow">
          <span class="lvl">{{ t('ceoboard.page.bannerRegistry') }}</span><span>{{ ov.registry_notice }}</span>
        </div>
      </div>
      <div v-if="ov.notice" class="ceo-alerts">
        <div class="ceo-alert yellow">
          <span class="lvl">{{ t('ceoboard.page.bannerPermission') }}</span><span>{{ ov.notice }}</span>
        </div>
      </div>

      <CeoAlertList :alerts="ov.alerts" />
      <CeoApprovalsPanel :approvals="ov.approvals" />
      <CeoLaneGroup v-for="lane in ov.lanes" :key="lane.biz_line" :lane="lane" :oncall="true" @open-node="openNode" />

      <div class="ceo-footer">
        {{ t('ceoboard.page.updatedAt', { at: updatedAt }) }} · {{ t('ceoboard.page.autoRefresh') }} ·
        {{
          t('ceoboard.page.registryLine', {
            dir: ov.registry.dir,
            loaded: ov.registry.loaded.length,
            failed: ov.registry.failed.length,
            skipped: ov.registry.skipped.length,
          })
        }}
        <span v-if="meName"> · {{ t('ceoboard.page.currentAccount', { name: meName, roles: meRoles }) }}</span>
      </div>
    </template>

    <CeoNodeDrawer v-if="drawer" :detail="drawer" @close="closeDrawer" />
  </q-page>
</template>
