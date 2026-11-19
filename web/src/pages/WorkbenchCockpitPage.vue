<template>
  <q-page class="wb-page" padding>
    <CockpitShell :retreat="!!drill" :offline="!wsConnected">
      <template #topbar>
        <CockpitTopBar
          :await-total="youQueue.total.value"
          :ws-connected="wsConnected"
          :clock-iso="nowIso"
          :trust-state="trustState"
          :stale-seconds="staleSeconds"
          @open-palette="paletteOpen = true"
          @refresh="onRefreshAll"
        />
      </template>

      <template #sidebar>
        <RosterSidebar
          :roster="pageState.roster.value"
          :live="stream.liveAgents"
          :waiting="waitingAgents"
          :biz-lines="bizLineCards"
          :loaded="pageState.rosterLoaded.value"
          @select="(key) => openDrillFor(key, 'dot')"
          @open-organization="goOrganization"
        />
      </template>

      <template #main>
        <OverviewGrid
          :await-count="youQueue.total.value"
          :agent-count="pageState.allAgents.value.length"
          :completions="completionsToday"
          :backfill-ready="pageState.backfillDone.value"
          :event-rate="stream.eventRatePerMin.value"
          :biz-lines="bizLineCards"
          :logs="stream.logLines.value"
          class="wb-page__overview"
        >
          <NodeMapCenter
            :roster="pageState.roster.value"
            :agents="pageState.allAgents.value"
            :live="stream.liveAgents"
            :waiting="waitingAgents"
            :sources="bizLineCards"
            :retreat="!!drill"
            @drill="openDrillFor"
            @open-organization="goOrganization"
          />
        </OverviewGrid>
      </template>

      <template #you>
        <YouDrawer
          :lanes="youQueue.lanes.value"
          :total="youQueue.total.value"
          :selected-id="floatItem?.id ?? ''"
          :busy-ids="busyIds"
          :errors="itemErrors"
          :now-iso="nowIso"
          @sign="onRowSign"
          @batch-sign="onBatchSign"
          @goto-item="onGotoItem"
          @focus-item="onFocusItem"
          @retry-lane="onRetryLane"
          @open-route="(route) => router.push(route)"
        />
      </template>

      <template #statusbar>
        <StatusBar
          :ws-connected="wsConnected"
          :clock-iso="nowIso"
          :freshness-seconds="laneFreshnessSeconds"
          :event-rate="stream.eventRatePerMin.value"
          :backlog-count="backlogCount"
          :trust-state="trustState"
        />
      </template>

      <template #floats>
        <ApprovalFloat
          :item="floatItem"
          :busy="!!floatItem && (busyIds.includes(floatItem.id) || busyIds.includes(`reject:${floatItem.id}`))"
          :docked="!!drill"
          :error="floatItem ? (itemErrors[floatItem.id] ?? '') : ''"
          @sign="onFloatSign"
          @dismiss="floatItem = null"
        />
        <SealStamp :receipt="sealReceipt" @done="onSealDone" />
        <AgentDrawer
          :target="drill"
          :live="drill ? stream.liveAgents.get(drill.agentKey) : undefined"
          @close="pageState.closeDrill"
        />
        <WbCommandPalette
          :open="paletteOpen"
          :items="paletteItems"
          @close="paletteOpen = false"
          @run="runCommand"
          @update:query="(q) => (paletteQuery = q)"
        />
      </template>
    </CockpitShell>
  </q-page>
</template>

<script setup lang="ts">
/**
 * 作战总台页面实例（设计方案 P0：晨检 + 签批闭环）。
 * 数据流铁律：本页只组装 features/workbench composables + components/workbench 展示族；
 * 跨域直读的 TECH-DEBT 注记收敛在 useYouQueue / useWorkbenchPage。
 */
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { useQuasar } from 'quasar';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import CockpitShell from '../components/workbench/CockpitShell.vue';
import CockpitTopBar from '../components/workbench/CockpitTopBar.vue';
import RosterSidebar from '../components/workbench/RosterSidebar.vue';
import OverviewGrid from '../components/workbench/OverviewGrid.vue';
import NodeMapCenter from '../components/workbench/NodeMapCenter.vue';
import YouDrawer from '../components/workbench/YouDrawer.vue';
import ApprovalFloat from '../components/workbench/ApprovalFloat.vue';
import SealStamp from '../components/workbench/SealStamp.vue';
import AgentDrawer from '../components/workbench/AgentDrawer.vue';
import StatusBar from '../components/workbench/StatusBar.vue';
import WbCommandPalette from '../components/workbench/WbCommandPalette.vue';
import { useYouQueue } from '../features/workbench/useYouQueue';
import { useWorkbenchStream, WS_STALE_MS } from '../features/workbench/useWorkbenchStream';
import { useWorkbenchPage, deriveTrustState } from '../features/workbench/useWorkbenchPage';
import {
  filterWbCommands,
  WB_COMMAND_DEFS,
  type WbCommandId,
  type WbCommandItem,
} from '../features/workbench/commands';
import type { DrillCard, SignReceipt, YouItem } from '../features/workbench/types';

const router = useRouter();
const { t } = useI18n();
const $q = useQuasar();

// ——— 组合 ———
const youQueue = useYouQueue();
const stream = useWorkbenchStream({ onConfirmActivity: () => youQueue.pokeFromWs() });
const pageState = useWorkbenchPage();

// ——— 时钟（秒级节拍；信任栏/时钟同源） ———
const nowIso = ref(new Date().toISOString());
let clockTimer: ReturnType<typeof setInterval> | null = null;

// ——— 信任六态（R1/R5 同源） ———
// stale = WS 静默窗（事件流沉默），不是 lane 新鲜度
const staleSeconds = computed(() => {
  const base = stream.lastEventAt.value;
  if (!base) return 0;
  const at = Date.parse(base);
  if (Number.isNaN(at)) return 0;
  return Math.max(0, Math.floor((Date.parse(nowIso.value) - at) / 1000));
});

const backlogCount = computed(() => youQueue.byLane.value.get('alerts')?.count ?? 0);

const trustState = computed(() =>
  deriveTrustState({
    wsConnected: stream.connected.value,
    staleMs: staleSeconds.value * 1000,
    wsStaleMs: WS_STALE_MS,
    lanesDegraded:
      youQueue.lanes.value.some((l) => l.degraded) || pageState.rosterFailed.value || pageState.backfillFailed.value,
    backlogCount: backlogCount.value,
    replayMode: false,
  }),
);

const wsConnected = computed(() => stream.connected.value);

// ——— YOU 金门 ———
const floatItem = ref<YouItem | null>(null);
/** 在途签批条目（含批量；reject: 前缀表示拒绝理由确认在途）——驱动行内 loading/disable 防重复提交 */
const busyIds = ref<string[]>([]);
/** 行内失败原因（不落戳不写绿行；刷新后仍在该条目则持续展示） */
const itemErrors = ref<Record<string, string>>({});
const sealReceipt = ref<SignReceipt | null>(null);

/** 状态栏新鲜度：各 lane 最旧一次成功刷新（与 YOU 行内 freshness 同源） */
const laneFreshnessSeconds = computed(() => {
  const oldest = youQueue.oldestUpdatedAt.value;
  if (!oldest) return -1;
  const at = Date.parse(oldest);
  if (Number.isNaN(at)) return -1;
  return Math.max(0, Math.floor((Date.parse(nowIso.value) - at) / 1000));
});

/** 等批归因集：WS confirm 命中 + lane 行 agentKey 命中（无 join key 的 lane 不强归因） */
const waitingAgents = computed(() => {
  const set = new Set<string>(stream.waitingAgents);
  for (const lane of youQueue.lanes.value) {
    for (const item of lane.items) {
      if (item.agentKey) set.add(item.agentKey);
    }
  }
  return set;
});

const bizLineCards = computed(() => pageState.buildBizLineCards(youQueue.lanes.value));

const completionsToday = computed(() => stream.completionsToday.value);

function onFocusItem(item: YouItem): void {
  // 无写路径的条目（stale 交易确认 / 非可决议审批类）浮层里没有动作可做，
  // 直接走深链，避免打开一个只能关闭的死胡同浮层。
  if (item.action === 'goto') {
    onGotoItem(item);
    return;
  }
  floatItem.value = item;
}

function onGotoItem(item: YouItem): void {
  if (item.to) void router.push(item.to);
}

async function applySign(
  item: YouItem,
  verb: 'approved' | 'rejected' | 'acked' | 'resolved',
  note: string,
): Promise<boolean> {
  if (busyIds.value.includes(item.id) || busyIds.value.includes(`reject:${item.id}`)) return false;
  const key = verb === 'rejected' ? `reject:${item.id}` : item.id;
  busyIds.value = [...busyIds.value, key];
  delete itemErrors.value[item.id];
  try {
    const receipt = await youQueue.sign(item, verb, note);
    if (receipt) {
      sealReceipt.value = receipt;
      stream.completions.value.push({ kind: 'seal', at: receipt.at, label: receipt.title });
      if (floatItem.value?.id === item.id) floatItem.value = null;
      return true;
    }
    return false;
  } catch (err) {
    // 异常合约（设计 §4.2）：行内 reason + 金日志记录（不欺骗）；不落戳不写绿行
    const reason = err instanceof Error ? err.message : String(err);
    itemErrors.value = { ...itemErrors.value, [item.id]: reason };
    stream.pushGoldLog(t('workbench.sign.failedLog', { title: item.summary.slice(0, 40), reason }));
    return false;
  } finally {
    busyIds.value = busyIds.value.filter((k) => k !== key);
  }
}

function onRowSign(item: YouItem, verb: 'approved' | 'rejected' | 'acked' | 'resolved', note = ''): void {
  void applySign(item, verb, note);
}

function onFloatSign(item: YouItem, verb: 'approved' | 'rejected' | 'acked' | 'resolved', note: string): void {
  void applySign(item, verb, note);
}

/** 批量签批：串行执行（避免同一时刻多次写），逐条失败行内反馈并汇总 */
async function onBatchSign(items: YouItem[], verb: 'approved' | 'acked'): Promise<void> {
  if (items.length === 0 || busyIds.value.length > 0) return;
  let ok = 0;
  let failed = 0;
  for (const item of items) {
    const done = await applySign(item, verb, '');
    if (done) ok += 1;
    else failed += 1;
  }
  if (failed > 0) $q.notify({ type: 'warning', message: t('workbench.you.batchResult', { ok, failed }) });
}

/** lane 级重试：失败注记不自动重试，用户显式触发一次全量刷新 */
function onRetryLane(): void {
  void youQueue.refresh();
}

function onSealDone(receipt: SignReceipt): void {
  // 条目坍缩为绿动词行：「HH:MM 你 批准了《X》」进日志卷宗
  const d = new Date(receipt.at);
  const hh = Number.isNaN(d.getTime())
    ? '--:--'
    : `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
  stream.pushSealLog(`${hh} ${t(`workbench.seal.${receipt.verb}`, { title: receipt.title.slice(0, 40) })}`);
  sealReceipt.value = null;
}

// 回执同熄：浮层条目在队列里消失（轮询对账后）时灭浮层，不做本地假熄灭
const floatStillPending = computed(() => {
  if (!floatItem.value) return false;
  return youQueue.lanes.value.some((l) => l.items.some((i) => i.id === floatItem.value?.id));
});
// 轮询刷新后校验（watch 在 setup 顶部无法 behind computed 依赖收集？用轮询守卫即可）
let floatGuardTimer: ReturnType<typeof setInterval> | null = null;

// ——— 下钻 ———
const drill = computed(() => pageState.drill.value);

function openDrillFor(agentKey: string, card: DrillCard): void {
  const agent = pageState.allAgents.value.find((a) => a.key === agentKey);
  pageState.openDrill({ agentKey, agentName: agent?.name ?? agentKey, card });
}

function goOrganization(): void {
  void router.push('/settings/organization');
}

// ——— ⌘K（P0 基础集） ———
const paletteOpen = ref(false);
const paletteQuery = ref('');

const paletteItems = computed<WbCommandItem[]>(() => {
  const items: WbCommandItem[] = WB_COMMAND_DEFS.filter((d) => d.id !== 'close-drill' || !!drill.value).map((def) => ({
    def,
    title: t(`workbench.cmd.${def.id}`),
  }));
  return filterWbCommands(items, paletteQuery.value);
});

function runCommand(id: WbCommandId): void {
  paletteOpen.value = false;
  switch (id) {
    case 'refresh':
      onRefreshAll();
      break;
    case 'focus-you': {
      const next = youQueue.lanes.value
        .flatMap((l) => l.items)
        .find((i) => i.action === 'approve' || i.action === 'ack');
      if (next) floatItem.value = next;
      break;
    }
    case 'goto-organization':
      goOrganization();
      break;
    case 'goto-sessions':
      void router.push('/sessions');
      break;
    case 'goto-cron':
      void router.push('/cron');
      break;
    case 'goto-overview':
      void router.push('/overview');
      break;
    case 'close-drill':
      pageState.closeDrill();
      break;
    default:
      break;
  }
}

function onKeydown(e: KeyboardEvent): void {
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
    e.preventDefault();
    paletteOpen.value = !paletteOpen.value;
    return;
  }
  if (e.key === 'Escape' && drill.value && !paletteOpen.value) pageState.closeDrill();
}

function onRefreshAll(): void {
  void youQueue.refresh();
  void pageState.loadRoster();
}

// ——— 生命周期 ———
onMounted(async () => {
  clockTimer = setInterval(() => {
    nowIso.value = new Date().toISOString();
  }, 1000);
  floatGuardTimer = setInterval(() => {
    if (floatItem.value && !floatStillPending.value) floatItem.value = null;
  }, 2000);
  window.addEventListener('keydown', onKeydown);
  void pageState.loadRoster();
  // 冷开 30s 非空保底（验收红线 3）+ 刷新后『你』落点仍在（验收红线 5）
  const backfill = await pageState.fetchDecisionBackfill();
  stream.backfill(backfill.lines, backfill.humanDoneCount);
});

onUnmounted(() => {
  if (clockTimer) clearInterval(clockTimer);
  if (floatGuardTimer) clearInterval(floatGuardTimer);
  window.removeEventListener('keydown', onKeydown);
});
</script>

<style scoped lang="sass">
.wb-page
  display: flex
  flex-direction: column
  min-height: 0
.wb-page__overview
  flex: 1
  min-height: 0
  padding: 10px 12px
</style>
