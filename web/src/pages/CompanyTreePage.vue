<template>
  <!--
    看板 › 总览（照设计稿，一屏）：一行结论；公司树直接画在页面上；树下面一条是选中节点的详情。
    数据只用已有的：组织架构、Agent 名册、待办、交办登记、定时任务（见右上角的说明）。
  -->
  <q-page class="app-standard-page co-page">
    <header class="co-head">
      <h1 class="co-head__line">
        <span class="co-head__name">{{ tree.company?.name || t('company.title') }}</span
        >{{ conclusion }}
      </h1>
      <q-select
        v-if="tree.companies.length > 1"
        v-model="companyId"
        class="co-head__company"
        dense
        outlined
        emit-value
        map-options
        :options="companyOptions"
        :label="t('company.pickCompany')"
      />
      <span v-if="updatedAt" class="co-head__fresh co-mono">
        {{ t('company.updatedAt', { time: shortTime(updatedAt, now) }) }}
      </span>
      <q-btn flat dense round icon="info" :aria-label="t('company.footnote')">
        <q-tooltip max-width="360px">{{ t('company.footnote') }}</q-tooltip>
      </q-btn>
      <q-btn flat dense no-caps icon="refresh" :label="t('company.refresh')" :loading="refreshing" @click="onRefresh" />
    </header>

    <p v-if="status === 'error'" class="co-note co-note--danger" role="alert">{{ t('company.loadFailed') }}</p>
    <p v-else-if="status === 'loading'" class="co-note" role="status">{{ t('company.loading') }}</p>
    <template v-else>
      <p v-if="partial.length" class="co-note co-note--warn" role="status">
        {{ t('company.partial', { sources: partial.map((s) => t(`company.source.${s}`)).join('、') }) }}
      </p>
      <p v-if="!tree.company" class="co-note">
        {{ t('company.noCompany') }}
        <router-link class="co-link" to="/settings/organization">{{ t('company.goOrg') }}</router-link>
      </p>

      <template v-else>
        <CompanyTree :tree="tree" :now="now" :selected="selected" @select="selected = $event" />

        <aside class="co-detail" aria-live="polite">
          <div>
            <header class="co-detail__head">
              <h2 class="co-detail__title">{{ detail.title }}</h2>
              <span v-if="detail.state" class="co-state" :class="`co-state--${detail.state}`">
                {{ t(`company.state.${detail.state}`) }}
              </span>
            </header>
            <p class="co-detail__now">{{ detail.now }}</p>
          </div>
          <dl class="co-detail__facts">
            <template v-for="fact in detail.facts" :key="fact.label">
              <dt>{{ fact.label }}</dt>
              <dd :class="{ 'co-mono': fact.mono }">{{ fact.value }}</dd>
            </template>
          </dl>
          <ul class="co-detail__agents">
            <li v-for="agent in detail.agents" :key="agent.id">
              <router-link class="co-detail__agent" :to="`/chat?agent=${encodeURIComponent(agent.id)}`">
                <span
                  class="co-dot"
                  :class="agent.failed ? 'co-dot--attention' : agent.ranToday ? 'co-dot--ok' : 'co-dot--idle'"
                />
                <span class="co-detail__agent-name">{{ agent.name }}</span>
                <span class="co-mono co-detail__agent-time">
                  {{
                    agent.failed ? t('company.agentFailed') : shortTime(agent.lastRunAt, now) || t('company.neverRan')
                  }}
                </span>
              </router-link>
            </li>
          </ul>
          <div class="co-detail__actions">
            <q-btn
              v-for="(action, index) in detail.actions"
              :key="action.to"
              no-caps
              dense
              :unelevated="index === 0"
              :outline="index > 0"
              color="primary"
              :label="action.label"
              :to="action.to"
            />
          </div>
        </aside>
      </template>
    </template>
  </q-page>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import CompanyTree from '../components/company/CompanyTree.vue';
import { shortTime, type NodeState, type TreeAgent } from '../features/company/tree';
import { useCompanyTree } from '../features/company/useCompanyTree';

type Fact = { label: string; value: string; mono?: boolean };
type Detail = {
  title: string;
  state: NodeState | '';
  now: string;
  facts: Fact[];
  agents: TreeAgent[];
  actions: { label: string; to: string }[];
};

const { t } = useI18n();
const { status, partial, updatedAt, nowIso, companyId, tree, refresh } = useCompanyTree();

/** 选中的节点：me / ceo / dept:<id> / pos:<deptId>:<posId> */
const selected = ref('me');
const refreshing = ref(false);
const now = computed(() => new Date(nowIso.value));

const companyOptions = computed(() => tree.value.companies.map((c) => ({ value: c.id, label: c.name })));
const selectedCompany = computed(() => tree.value.company?.id ?? '');
// 选择器显示当前实际选中的公司（没存过、或保存的 id 失效时回落到第一家）。
watch(selectedCompany, (id) => {
  if (id && companyId.value !== id) companyId.value = id;
});
// 换公司后原来选中的部门不在了，回到「我」。
watch(companyId, () => (selected.value = 'me'));

const conclusion = computed(() => {
  if (status.value !== 'ok' || !tree.value.company) return '';
  const hot = tree.value.depts.filter((d) => d.state === 'attention');
  const { waiting } = tree.value.me;
  if (hot.length) {
    const names = hot
      .slice(0, 3)
      .map((d) => d.name)
      .join('、');
    return t('company.conclusion.attention', { n: hot.length, names, waiting });
  }
  if (waiting > 0) return t('company.conclusion.waiting', { waiting });
  if (tree.value.totals.activeToday === 0) return t('company.conclusion.quiet');
  return t('company.conclusion.ok', { active: tree.value.totals.activeToday });
});

const time = (iso: string) => shortTime(iso, now.value) || t('company.none');

const detail = computed<Detail>(() => {
  const data = tree.value;
  const [kind, deptId, posId] = selected.value.split(':');
  const dept = data.depts.find((d) => d.id === deptId);
  if (kind === 'ceo') {
    if (!data.ceo) {
      return {
        title: t('company.ceo.title'),
        state: '',
        now: t(data.ceoConfigured ? 'company.ceo.missing' : 'company.ceo.unset'),
        facts: [],
        agents: [],
        actions: [{ label: t('company.goOrg'), to: '/settings/organization' }],
      };
    }
    return {
      title: data.ceo.name,
      state: data.ceo.failed ? 'attention' : data.ceo.ranToday ? 'ok' : 'idle',
      now: t('company.ceo.role'),
      facts: [{ label: t('company.fact.lastRun'), value: time(data.ceo.lastRunAt), mono: true }],
      agents: [],
      actions: [{ label: t('company.openChat'), to: `/chat?agent=${encodeURIComponent(data.ceo.id)}` }],
    };
  }
  if (kind === 'dept' && dept) {
    return {
      title: dept.name,
      state: dept.state,
      now: t(`company.deptNow.${dept.state}`, { failed: dept.failed, waiting: dept.waiting, active: dept.activeToday }),
      facts: [
        {
          label: t('company.fact.lead'),
          value: dept.lead?.name ?? t(dept.leadMissing ? 'company.leadMissing' : 'company.leadUnset'),
        },
        { label: t('company.fact.agents'), value: String(dept.agentCount), mono: true },
        { label: t('company.fact.delegated'), value: String(dept.delegated), mono: true },
        { label: t('company.fact.lastRun'), value: time(dept.lastRunAt), mono: true },
        { label: t('company.fact.nextRun'), value: time(dept.nextRunAt), mono: true },
      ],
      agents: dept.lead ? [dept.lead] : [],
      actions: [
        ...(dept.waiting > 0 ? [{ label: t('company.goDecide'), to: '/workbench' }] : []),
        ...(dept.lead ? [{ label: t('company.askLead'), to: `/chat?agent=${encodeURIComponent(dept.lead.id)}` }] : []),
      ],
    };
  }
  const pos = kind === 'pos' ? dept?.positions.find((p) => p.id === posId) : undefined;
  if (pos && dept) {
    return {
      title: `${dept.name} · ${pos.name}`,
      state: pos.state,
      now: t(`company.posNow.${pos.state}`, { failed: pos.failed, waiting: pos.waiting, n: pos.agents.length }),
      facts: [],
      agents: pos.agents,
      actions: pos.waiting > 0 ? [{ label: t('company.goDecide'), to: '/workbench' }] : [],
    };
  }
  return {
    title: t('company.me.title'),
    state: data.me.waiting > 0 ? 'attention' : 'ok',
    now:
      data.me.waiting > 0
        ? t('company.me.waiting', { n: data.me.waiting, high: data.me.highRisk })
        : t('company.me.clear'),
    facts: data.me.earliestDue
      ? [{ label: t('company.fact.earliestDue'), value: time(data.me.earliestDue), mono: true }]
      : [],
    agents: [],
    actions: [{ label: t(data.me.waiting > 0 ? 'company.goDecide' : 'company.goWorkbench'), to: '/workbench' }],
  };
});

async function onRefresh() {
  refreshing.value = true;
  await refresh();
  refreshing.value = false;
}
</script>
