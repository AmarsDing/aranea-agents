<script setup lang="ts">
// Harness › 运行与治理 › 流程健康（/ceo/oncall，2026-11-19 由「值班视图」重排）：
// 回答「外部业务流程的节点是否通畅」。后端没启用流程看板时（404）给接入引导，不显示成故障；
// 启用后：四格状态 → 页签（流程 / 死信与重放 / 消费者 / 注册表）。老板视图在 看板 › 总览，这里只放一个链接。
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import AppPageHero from '../components/layout/AppPageHero.vue';
import OpsGuide from '../components/ops/OpsGuide.vue';
import CeoPlatformChips from '../components/ceoboard/CeoPlatformChips.vue';
import CeoAlertList from '../components/ceoboard/CeoAlertList.vue';
import CeoApprovalsPanel from '../components/ceoboard/CeoApprovalsPanel.vue';
import CeoLaneGroup from '../components/ceoboard/CeoLaneGroup.vue';
import CeoNodeDrawer from '../components/ceoboard/CeoNodeDrawer.vue';
import { useCeoboard } from '../features/ceoboard/useCeoboard';
import { useFlowOps } from '../features/ceoboard/useFlowOps';
import { confirm } from '../composables/useAppConfirm';
import { relativeTime } from '../features/monitor/eventView';

const { t } = useI18n();
const $q = useQuasar();
const { ov, err, notConfigured, drawer, refresh, openNode, closeDrawer } = useCeoboard();
const ops = useFlowOps();
const { deadLetters, deadPending, deadState, consumers, consumerState, backlog, disconnected, replaying } = ops;

const tab = ref<'flows' | 'dead' | 'consumers' | 'registry'>('flows');

// 页签切过去才读；流程总览本身 15 秒轮询（useCeoboard）
watch(tab, (v) => {
  if (v === 'dead') void ops.loadDeadLetters();
  if (v === 'consumers') void ops.loadConsumers();
});
// 首次接通后顺手读一次死信与消费者，状态格里的数才不是「—」
watch(
  () => Boolean(ov.value),
  (ready) => {
    if (!ready) return;
    void ops.loadDeadLetters();
    void ops.loadConsumers();
  },
);

const workflows = computed(() => ov.value?.lanes.flatMap((l) => l.workflows) ?? []);
const badFlows = computed(() => workflows.value.filter((w) => w.health === 'red' || w.health === 'yellow').length);

type Tone = 'ok' | 'watch' | 'alert' | 'off';
const kpis = computed(() => {
  const none = t('ops.common.none');
  const alerts = ov.value?.alerts.length ?? 0;
  const approvals = ov.value?.approvals.count ?? 0;
  return [
    {
      key: 'flows',
      label: t('ops.flows.kpi.flows'),
      value: String(workflows.value.length),
      sub: t('ops.flows.kpi.flowsSub', { bad: badFlows.value }),
      tone: (badFlows.value ? 'watch' : 'ok') as Tone,
    },
    {
      key: 'alerts',
      label: t('ops.flows.kpi.alerts'),
      value: String(alerts),
      sub: t('ops.flows.kpi.approvalsSub', { n: approvals }),
      tone: (alerts ? 'alert' : 'ok') as Tone,
    },
    {
      key: 'dead',
      label: t('ops.flows.kpi.dead'),
      value: deadState.value.forbidden || deadState.value.error ? none : String(deadPending.value),
      sub: deadState.value.forbidden
        ? t('ops.flows.noPermission')
        : t('ops.flows.kpi.deadSub', { n: deadLetters.value.length }),
      tone: (deadPending.value ? 'alert' : deadState.value.forbidden ? 'off' : 'ok') as Tone,
    },
    {
      key: 'consumers',
      label: t('ops.flows.kpi.backlog'),
      value: consumerState.value.error ? none : String(backlog.value),
      sub: t('ops.flows.kpi.backlogSub', { n: consumers.value.length, off: disconnected.value }),
      tone: (disconnected.value ? 'alert' : backlog.value ? 'watch' : 'ok') as Tone,
    },
  ];
});

const registry = computed(() => ov.value?.registry ?? null);

async function onReplay(id: number, refKey: string) {
  const ok = await confirm({
    title: t('ops.flows.dead.replayTitle'),
    message: t('ops.flows.dead.replayMessage', { key: refKey || id }),
    cancelLabel: t('common.cancel'),
    confirmLabel: t('ops.flows.dead.replay'),
  });
  if (!ok) return;
  const res = await ops.replay(id);
  if (res.error) $q.notify({ type: 'negative', message: t('ops.flows.dead.replayFailed', { msg: res.error }) });
  else
    $q.notify({ type: 'positive', message: res.deduped ? t('ops.flows.dead.deduped') : t('ops.flows.dead.replayed') });
}

function refreshAll() {
  void refresh();
  if (tab.value === 'dead') void ops.loadDeadLetters();
  if (tab.value === 'consumers') void ops.loadConsumers();
}
</script>

<template>
  <q-page class="app-standard-page op-page ceo-page">
    <AppPageHero :title="t('ops.flows.title')" :subtitle="t('ops.flows.hint')">
      <template #actions>
        <q-btn flat no-caps icon="account_tree" :label="t('ops.flows.bossView')" to="/ceo/overview" />
        <q-btn flat round dense icon="refresh" :aria-label="t('ops.common.refresh')" @click="refreshAll" />
      </template>
    </AppPageHero>

    <OpsGuide
      v-if="notConfigured"
      data-state="empty"
      icon="device_hub"
      :title="t('ops.flows.guide.title')"
      :description="t('ops.flows.guide.desc')"
      :steps="[t('ops.flows.guide.step1'), t('ops.flows.guide.step2'), t('ops.flows.guide.step3')]"
    />

    <p v-else-if="err" class="op-note op-note--danger" role="alert">{{ err }}</p>

    <template v-if="ov && !notConfigured">
      <div class="op-kpis" role="list">
        <div v-for="k in kpis" :key="k.key" class="op-kpi" :class="`op-kpi--${k.tone}`" role="listitem">
          <span class="op-kpi__label">{{ k.label }}</span>
          <span class="op-kpi__value op-num">{{ k.value }}</span>
          <span class="op-kpi__sub">{{ k.sub }}</span>
        </div>
      </div>

      <div v-if="ov.registry_notice || ov.notice" class="ceo-alerts">
        <div v-if="ov.registry_notice" class="ceo-alert yellow">
          <span class="lvl">{{ t('ceoboard.page.bannerRegistry') }}</span
          ><span>{{ ov.registry_notice }}</span>
        </div>
        <div v-if="ov.notice" class="ceo-alert yellow">
          <span class="lvl">{{ t('ceoboard.page.bannerPermission') }}</span
          ><span>{{ ov.notice }}</span>
        </div>
      </div>

      <q-tabs v-model="tab" class="op-tabs" align="left" no-caps dense inline-label>
        <q-tab name="flows" icon="account_tree" :label="t('ops.flows.tab.flows')" />
        <q-tab name="dead" icon="replay" :label="t('ops.flows.tab.dead')" />
        <q-tab name="consumers" icon="hub" :label="t('ops.flows.tab.consumers')" />
        <q-tab name="registry" icon="folder_open" :label="t('ops.flows.tab.registry')" />
      </q-tabs>

      <section v-if="tab === 'flows'" class="op-flows" :aria-label="t('ops.flows.tab.flows')">
        <CeoPlatformChips :platforms="ov.platforms" />
        <CeoAlertList :alerts="ov.alerts" />
        <CeoApprovalsPanel :approvals="ov.approvals" />
        <CeoLaneGroup v-for="lane in ov.lanes" :key="lane.biz_line" :lane="lane" :oncall="true" @open-node="openNode" />
        <p v-if="!ov.lanes.length" class="op-empty">{{ t('ops.flows.noFlows') }}</p>
      </section>

      <section v-else-if="tab === 'dead'" class="op-panel" :aria-label="t('ops.flows.tab.dead')">
        <header class="op-panel__head">
          <h2 class="op-panel__title">
            {{ t('ops.flows.tab.dead') }}<span class="op-panel__count op-num">{{ deadPending }}</span>
          </h2>
          <span class="op-panel__meta">{{ t('ops.flows.dead.hint') }}</span>
        </header>
        <p v-if="deadState.forbidden" class="op-empty">{{ t('ops.flows.dead.forbidden') }}</p>
        <p v-else-if="deadState.error" class="op-note op-note--danger" role="alert">
          {{ t('ops.common.loadFailed', { msg: deadState.error }) }}
        </p>
        <p v-else-if="!deadLetters.length && !deadState.loading" class="op-empty">{{ t('ops.flows.dead.empty') }}</p>
        <ul v-else class="op-list">
          <li v-for="d in deadLetters" :key="d.id" class="op-row">
            <span class="op-dot" :class="d.replayed_at ? 'op-dot--ok' : 'op-dot--alert'" aria-hidden="true" />
            <span class="op-row__main">
              <span class="op-row__title op-num">{{ d.source }} · {{ d.kind }} · {{ d.ref_key }}</span>
              <span class="op-row__sub" :title="d.payload">{{ d.reason }}</span>
            </span>
            <span class="op-row__time op-num">{{ relativeTime(t, d.failed_at) }}</span>
            <span v-if="d.replayed_at" class="op-tag op-tag--ok">{{ t('ops.flows.dead.done') }}</span>
            <q-btn
              v-else
              outline
              dense
              no-caps
              color="primary"
              icon="replay"
              :label="t('ops.flows.dead.replay')"
              :loading="replaying === d.id"
              @click="onReplay(d.id, d.ref_key)"
            />
          </li>
        </ul>
      </section>

      <section v-else-if="tab === 'consumers'" class="op-panel" :aria-label="t('ops.flows.tab.consumers')">
        <header class="op-panel__head">
          <h2 class="op-panel__title">
            {{ t('ops.flows.tab.consumers') }}<span class="op-panel__count op-num">{{ consumers.length }}</span>
          </h2>
          <span class="op-panel__meta">{{ t('ops.flows.consumers.hint') }}</span>
        </header>
        <p v-if="consumerState.error" class="op-note op-note--danger" role="alert">
          {{ t('ops.common.loadFailed', { msg: consumerState.error }) }}
        </p>
        <p v-else-if="!consumers.length && !consumerState.loading" class="op-empty">
          {{ t('ops.flows.consumers.empty') }}
        </p>
        <ul v-else class="op-list">
          <li v-for="c in consumers" :key="c.key" class="op-row">
            <span class="op-dot" :class="c.connected ? 'op-dot--ok' : 'op-dot--alert'" aria-hidden="true" />
            <span class="op-row__main">
              <span class="op-row__title op-num">{{ c.platform }} · {{ c.stream }} / {{ c.durable }}</span>
              <span class="op-row__sub">{{ c.error || c.subject || c.node }}</span>
            </span>
            <span class="op-tag" :class="c.connected ? 'op-tag--ok' : 'op-tag--alert'">
              {{ c.connected ? t('ops.flows.consumers.connected') : t('ops.flows.consumers.disconnected') }}
            </span>
            <span class="op-row__num op-num">{{ t('ops.flows.consumers.pending', { n: c.pending }) }}</span>
          </li>
        </ul>
      </section>

      <section v-else-if="registry" class="op-panel" :aria-label="t('ops.flows.tab.registry')">
        <header class="op-panel__head">
          <h2 class="op-panel__title">{{ t('ops.flows.tab.registry') }}</h2>
          <span class="op-panel__meta op-num">{{ registry.dir }}</span>
        </header>
        <h3 class="op-panel__sub">{{ t('ops.flows.registry.loaded', { n: registry.loaded.length }) }}</h3>
        <ul v-if="registry.loaded.length" class="op-chips">
          <li v-for="f in registry.loaded" :key="f" class="op-tag op-tag--ok op-num">{{ f }}</li>
        </ul>
        <template v-if="registry.failed.length">
          <h3 class="op-panel__sub">{{ t('ops.flows.registry.failed', { n: registry.failed.length }) }}</h3>
          <ul class="op-list">
            <li v-for="f in registry.failed" :key="f.file" class="op-row">
              <span class="op-dot op-dot--alert" aria-hidden="true" />
              <span class="op-row__main">
                <span class="op-row__title op-num">{{ f.file }}</span>
                <span class="op-row__sub">{{ f.errors.join('；') }}</span>
              </span>
            </li>
          </ul>
        </template>
        <template v-if="registry.skipped.length">
          <h3 class="op-panel__sub">{{ t('ops.flows.registry.skipped', { n: registry.skipped.length }) }}</h3>
          <ul class="op-list">
            <li v-for="(k, i) in registry.skipped" :key="i" class="op-row">
              <span class="op-dot op-dot--watch" aria-hidden="true" />
              <span class="op-row__main">
                <span class="op-row__title op-num">{{
                  [k.workflow, k.node, k.metric].filter(Boolean).join(' / ')
                }}</span>
                <span class="op-row__sub">{{ k.field }} · {{ k.reason }}</span>
              </span>
            </li>
          </ul>
        </template>
      </section>

      <p class="op-note op-num">{{ t('ops.flows.asOf', { time: relativeTime(t, ov.as_of) }) }}</p>
    </template>

    <CeoNodeDrawer v-if="drawer" :detail="drawer" @close="closeDrawer" />
  </q-page>
</template>
