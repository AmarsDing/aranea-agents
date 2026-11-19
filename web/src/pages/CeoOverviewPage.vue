<script setup lang="ts">
// /ceo/overview 老板视图（整合 P2-8）：结论先行 —— 总览 → 需要我处理 → 业务线卡片。
// 与值班视图共用同一份物化数据口径（同一 GetOverview RPC），仅布局不同。
// 2026-11-10：业务线区新增「公司运转图」模式（只读），与卡片视图切换。
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import { useAuthStore } from '../stores/auth';
import { useCeoboard } from '../features/ceoboard/useCeoboard';
import CeoViewSwitch from '../components/ceoboard/CeoViewSwitch.vue';
import CeoPlatformChips from '../components/ceoboard/CeoPlatformChips.vue';
import CeoHeroBanner from '../components/ceoboard/CeoHeroBanner.vue';
import CeoBusinessPulse from '../components/ceoboard/CeoBusinessPulse.vue';
import CeoDecisionPanel from '../components/ceoboard/CeoDecisionPanel.vue';
import CeoBizLaneCard from '../components/ceoboard/CeoBizLaneCard.vue';
import CeoCompanyGraph from '../components/ceoboard/CeoCompanyGraph.vue';
import CeoOpsGraph from '../components/ceoboard/CeoOpsGraph.vue';
import CeoNodeDrawer from '../components/ceoboard/CeoNodeDrawer.vue';

const { t } = useI18n();
const auth = useAuthStore();
const router = useRouter();
const { ov, snap, err, snapErr, drawer, openNode, closeDrawer } = useCeoboard();

const meName = computed(() => auth.user?.name?.trim() || auth.displayLabel || undefined);
const updatedAt = computed(() => {
  const at = snap.value?.as_of || ov.value?.as_of;
  return at ? new Date(at).toLocaleString() : '';
});

type ViewMode = 'ops' | 'cards' | 'flows';

const VIEW_MODE_KEY = 'aranea:ceoboard-overview-mode:v1';
function loadViewMode(): ViewMode {
  try {
    const saved = localStorage.getItem(VIEW_MODE_KEY);
    if (saved === 'cards' || saved === 'flows' || saved === 'ops') return saved;
    // 旧版本把流程拓扑存为 graph；迁移为 flows，新用户默认经营运转图。
    if (saved === 'graph') return 'flows';
  } catch {
    // 隐私模式读取失败时使用默认视图
  }
  return 'ops';
}
const viewMode = ref<ViewMode>(loadViewMode());
watch(viewMode, (v) => {
  try {
    localStorage.setItem(VIEW_MODE_KEY, v);
  } catch {
    // 隐私模式写入失败不阻断视图切换
  }
});

function drill(path: string) {
  if (path) void router.push(path);
}

const decidePanel = ref<InstanceType<typeof CeoDecisionPanel> | null>(null);
function showDecisions() {
  const el = decidePanel.value?.$el as HTMLElement | undefined;
  if (!el) return;
  const reduced =
    window.matchMedia('(prefers-reduced-motion: reduce)').matches ||
    document.documentElement.dataset.motion === 'reduced';
  el.scrollIntoView({ behavior: reduced ? 'auto' : 'smooth', block: 'start' });
}
</script>

<template>
  <q-page class="ceo-page">
    <div class="ceo-page-bar">
      <CeoViewSwitch />
      <CeoPlatformChips v-if="ov" :platforms="ov.platforms" />
    </div>

    <div v-if="err" class="ceo-alerts" role="alert">
      <div class="ceo-alert red">
        <span class="lvl">{{ t('ceoboard.page.bannerBoard') }}</span
        ><span>{{ err }}</span>
      </div>
    </div>
    <div v-if="snapErr" class="ceo-alerts" role="status">
      <div class="ceo-alert yellow">
        <span class="lvl">{{ t('ceoboard.ops.bannerOps') }}</span
        ><span>{{ snapErr }}</span>
      </div>
    </div>

    <template v-if="ov || snap">
      <CeoHeroBanner v-if="ov" :ov="ov" :snap="snap" :me-name="meName" />
      <CeoBusinessPulse v-if="snap" :snap="snap" @drill="drill" />
      <CeoDecisionPanel
        v-if="ov"
        ref="decidePanel"
        :ov="ov"
        :business-alerts="snap?.anomalies"
        :business-empty="snap?.company.health === 'gray' && !snap?.anomalies?.length"
        @drill="drill"
      />
      <div class="ceo-graph-switchbar">
        <span class="ceo-graph-switchbar__label">{{ t('ceoboard.ops.viewBar') }}</span>
        <div class="ceo-viewswitch" role="group" :aria-label="t('ceoboard.graph.viewLabel')">
          <button
            type="button"
            :class="{ on: viewMode === 'ops' }"
            :aria-pressed="viewMode === 'ops'"
            @click="viewMode = 'ops'"
          >
            {{ t('ceoboard.graph.viewOps') }}
          </button>
          <button
            type="button"
            :class="{ on: viewMode === 'cards' }"
            :aria-pressed="viewMode === 'cards'"
            :disabled="!ov"
            @click="viewMode = 'cards'"
          >
            {{ t('ceoboard.graph.viewCards') }}
          </button>
          <button
            type="button"
            :class="{ on: viewMode === 'flows' }"
            :aria-pressed="viewMode === 'flows'"
            :disabled="!ov"
            @click="viewMode = 'flows'"
          >
            {{ t('ceoboard.graph.viewGraph') }}
          </button>
        </div>
      </div>
      <!-- 经营运转图：真实电商业务实体（内容/评审/发布/商机/供应商/Agent）。 -->
      <CeoOpsGraph v-if="snap" v-show="viewMode === 'ops'" :snap="snap" @drill="drill" />
      <div v-if="!snap && viewMode === 'ops'" class="ceo-decide-ok ceo-ops-empty">
        {{ t('ceoboard.ops.empty') }}
      </div>
      <!-- 业务线卡片与流程拓扑：沿用原有物化口径，作为第二/第三视图。 -->
      <div v-if="viewMode === 'cards' && ov?.lanes?.length" class="ceo-biz-grid">
        <CeoBizLaneCard v-for="lane in ov.lanes" :key="lane.biz_line" :lane="lane" @open-node="openNode" />
      </div>
      <div v-else-if="viewMode === 'cards'" class="ceo-decide-ok ceo-ops-empty">
        {{ t('ceoboard.ops.noLanes') }}
      </div>
      <CeoCompanyGraph
        v-if="ov && viewMode === 'flows' && ov.lanes.length"
        :ov="ov"
        :me-name="meName"
        @open-node="openNode"
        @show-decisions="showDecisions"
      />
      <div v-else-if="viewMode === 'flows'" class="ceo-decide-ok ceo-ops-empty">
        {{ t('ceoboard.ops.noLanes') }}
      </div>
      <div class="ceo-footer">
        {{ t('ceoboard.page.updatedAt', { at: updatedAt }) }} · {{ t('ceoboard.page.autoRefresh') }} ·
        {{ t('ceoboard.ops.dataSource') }}
      </div>
    </template>

    <CeoNodeDrawer v-if="drawer" :detail="drawer" @close="closeDrawer" />
  </q-page>
</template>
