<script setup lang="ts">
// 决策区（整合 P2-8，自 ceo-dashboard DecisionPanel.vue 移植）：
// CEO 第二层信息——「需要我处理什么」。
// 左列：异常（翻译成业务语言的告警）；右列：等你拍板（审批）。
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { OpsAnomaly, Overview } from '../../features/ceoboard/types';
import { friendlyAlert } from '../../features/ceoboard/friendly';

const props = defineProps<{ ov: Overview; businessAlerts?: OpsAnomaly[]; businessEmpty?: boolean }>();
const emit = defineEmits<{ drill: [path: string] }>();
const { t } = useI18n();

interface AttentionItem {
  id: string;
  level: string;
  title: string;
  detail: string;
  since?: string;
  path?: string;
}

function anomalyText(a: OpsAnomaly): string {
  const key = `ceoboard.ops.anomalies.${a.code}`;
  const translated = t(key, a.params ?? {});
  return translated === key ? a.text : translated;
}

const attention = computed<AttentionItem[]>(() => {
  const biz = (props.businessAlerts ?? []).map((a) => ({
    id: `ops:${a.id}`,
    level: a.level,
    title: t(`ceoboard.ops.moduleNames.${a.module}`),
    detail: anomalyText(a),
    since: a.since,
    path: a.drill?.path,
  }));
  const infra = props.ov.alerts.map((a) => {
    const f = friendlyAlert(a);
    return { id: `infra:${a.id}`, level: a.level, title: f.title, detail: f.detail, since: a.since, path: undefined };
  });
  return [...biz, ...infra];
});

const approvals = computed(() => props.ov.approvals);

function fmtSince(at?: string): string {
  if (!at) return '';
  const d = new Date(at);
  const mins = Math.max(0, Math.round((Date.now() - d.getTime()) / 60000));
  if (mins < 1) return t('ceoboard.decide.sinceJust');
  if (mins < 60) return t('ceoboard.decide.sinceMinutes', { n: mins });
  if (mins < 1440) return t('ceoboard.decide.sinceHours', { n: Math.floor(mins / 60) });
  return t('ceoboard.decide.sinceDays', { n: Math.floor(mins / 1440) });
}
function fmt(at?: string): string {
  if (!at || new Date(at).getFullYear() < 2000) return t('ceoboard.common.neverOk');
  return new Date(at).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
}
function freshness(): string {
  const ok = approvals.value.last_ok_at && new Date(approvals.value.last_ok_at).getFullYear() >= 2000;
  return ok ? t('ceoboard.decide.approvalsFresh', { at: fmt(approvals.value.last_ok_at) }) : t('ceoboard.decide.approvalsUnavailable');
}
</script>

<template>
  <section class="ceo-decide">
    <div class="ceo-decide-col" :class="{ attention: attention.length > 0 }">
      <div class="ceo-decide-head">
        <h2>{{ t('ceoboard.decide.attention') }}</h2>
        <span class="ceo-decide-count" :class="attention.length ? '' : 'ok'">{{ attention.length }}</span>
      </div>
      <div v-if="!attention.length" class="ceo-decide-ok" :class="{ idle: businessEmpty }">
        {{ businessEmpty ? t('ceoboard.ops.decisionIdle') : `✓ ${t('ceoboard.decide.allOk')}` }}
      </div>
      <component
        :is="a.path ? 'button' : 'div'"
        v-for="a in attention"
        :key="a.id"
        class="ceo-decide-item"
        :class="{ 'is-link': !!a.path }"
        :type="a.path ? 'button' : undefined"
        :title="a.path ? `${a.detail} · ${t('ceoboard.ops.openDrill')}` : a.detail"
        @click="a.path ? emit('drill', a.path) : undefined"
      >
        <span class="ceo-dot ceo-dot--inline" :class="a.level"></span>
        <div class="body">
          <div class="t">{{ a.title }}</div>
          <div class="d">{{ a.detail }}</div>
        </div>
        <span class="when">{{ fmtSince(a.since) }}</span>
      </component>
    </div>

    <div class="ceo-decide-col">
      <div class="ceo-decide-head">
        <h2>{{ t('ceoboard.decide.approvals') }}</h2>
        <span class="ceo-decide-count" :class="approvals.count ? 'amber' : 'ok'">{{ approvals.count }}</span>
        <span class="ceo-dim ceo-freshness">· {{ freshness() }}</span>
      </div>
      <div v-if="approvals.note" class="ceo-decide-item">
        <span class="ceo-dot ceo-dot--inline yellow"></span>
        <div class="body">
          <div class="t">{{ t('ceoboard.decide.approvalsNoteTitle') }}</div>
          <div class="d">{{ approvals.note }}</div>
        </div>
      </div>
      <div v-else-if="!approvals.items.length" class="ceo-decide-ok">✓ {{ t('ceoboard.decide.approvalsEmpty') }}</div>
      <div v-for="(it, i) in approvals.items" :key="it.lane + it.id + i" class="ceo-decide-item">
        <span class="ceo-dot ceo-dot--inline yellow"></span>
        <div class="body">
          <div class="t">{{ it.title || it.id }}</div>
          <div class="d">
            {{ it.lane === 'publish' ? t('ceoboard.decide.lanePublish') : t('ceoboard.decide.laneListing')
            }}<template v-if="it.extra"> · {{ it.extra }}</template>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
