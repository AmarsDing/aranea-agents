<template>
  <!-- Harness › 连接 › 出站推送：关键数 → 筛选 → 构件卡 + 检视面板；还没有推送时给接入引导 -->
  <q-page class="app-standard-page cap-page webhooks-page">
    <Teleport defer to="#app-tab-actions">
      <q-btn
        v-if="rows.length"
        unelevated
        no-caps
        color="primary"
        icon="add"
        :label="t('connect.webhooks.create')"
        @click="openCreate"
      />
    </Teleport>

    <AppStatePanel v-if="forbidden" variant="forbidden" />
    <AppStatePanel v-else-if="!loaded && loading" variant="loading" :rows="4" />
    <AppStatePanel
      v-else-if="error && !rows.length"
      variant="error"
      :description="error"
      :action-label="t('webhooksPage.retry')"
      action-icon="refresh"
      @action="loadRows"
    />

    <ConnectGuide
      v-else-if="!rows.length"
      icon="send"
      :title="t('connect.webhooks.guideTitle')"
      :description="t('connect.webhooks.guideDesc')"
      :steps="[t('connect.webhooks.guideStep1'), t('connect.webhooks.guideStep2'), t('connect.webhooks.guideStep3')]"
      :action-label="t('connect.webhooks.create')"
      @action="openCreate"
    >
      <div class="cx-picks__group">
        <span class="cx-picks__group-title">{{ t('connect.webhooks.guideEvents') }}</span>
        <ul class="cap-card__tags" role="list">
          <li v-for="et in WEBHOOK_EVENT_TYPES" :key="et.value" class="cap-tag">{{ t(et.labelKey) }}</li>
        </ul>
      </div>
    </ConnectGuide>

    <template v-else>
      <AppStatePanel
        v-if="error"
        variant="partial"
        dense
        :description="error"
        :action-label="t('webhooksPage.retry')"
        action-icon="refresh"
        @action="loadRows"
      />
      <CapStats
        :stats="stats"
        :label="t('connect.common.stripLabel')"
        :active="stateFilter"
        @pick="stateFilter = stateFilter === $event ? '' : $event"
      />
      <div class="cap-bar">
        <q-input
          v-model="search"
          class="cap-bar__search"
          dense
          outlined
          clearable
          debounce="150"
          :placeholder="t('connect.webhooks.search')"
          :aria-label="t('connect.webhooks.search')"
        >
          <template #prepend><q-icon name="search" /></template>
        </q-input>
        <span class="cap-bar__spacer" />
        <span class="cap-bar__meta cx-num">{{
          t('connect.common.shownOf', { shown: total, total: rows.length })
        }}</span>
        <q-btn
          flat
          dense
          round
          icon="refresh"
          :loading="loading"
          :aria-label="t('connect.common.refresh')"
          @click="loadRows"
        />
      </div>

      <div class="cx-workspace" :class="{ 'cx-workspace--inspecting': inspectorOpen }">
        <div class="cx-workspace__main">
          <p v-if="!pagedRows.length" class="cap-note cap-note--info">
            <q-icon name="filter_alt_off" size="16px" />
            <span class="cap-note__body">{{ t('connect.common.noMatch') }}</span>
          </p>
          <div v-else class="cap-grid">
            <CapCard
              v-for="row in pagedRows"
              :key="row.id"
              icon="send"
              :code="hostOf(row.url)"
              :title="row.name"
              :description="row.url"
              :status="row.enabled ? statusOn : statusOff"
              :facts="cardFacts(row)"
              :tags="cardTags(row)"
              :selected="inspected?.id === row.id"
              @open="inspect(row)"
            >
              <template #switch>
                <q-toggle
                  :model-value="row.enabled"
                  dense
                  :disable="busyId === row.id"
                  :aria-label="row.enabled ? t('connect.common.disable') : t('connect.common.enable')"
                  @update:model-value="toggleEnabled(row, Boolean($event))"
                />
              </template>
              <template #relation>{{ t('connect.common.updatedAt') }} {{ formatTime(row.updated_at) }}</template>
              <template #actions>
                <q-btn
                  flat
                  dense
                  no-caps
                  icon="send"
                  :label="t('connect.webhooks.testRun')"
                  :loading="testingId === row.id"
                  :disable="testingId !== '' && testingId !== row.id"
                  @click="testRow(row)"
                />
                <q-btn flat dense round icon="more_horiz" :aria-label="t('connect.common.more')">
                  <q-menu anchor="bottom right" self="top right">
                    <q-list dense>
                      <q-item v-close-popup clickable @click="openEdit(row)">
                        <q-item-section>{{ t('connect.common.edit') }}</q-item-section>
                      </q-item>
                      <q-item v-close-popup clickable @click="confirmDelete(row)">
                        <q-item-section class="text-negative">{{ t('connect.common.remove') }}</q-item-section>
                      </q-item>
                    </q-list>
                  </q-menu>
                </q-btn>
              </template>
            </CapCard>
          </div>
          <AppRegistryPagination
            v-if="total > pageSize"
            v-model:page="page"
            v-model:page-size="pageSize"
            :page-max="pageMax"
            :total="total"
            :loading="loading"
            :label="t('webhooksPage.paginationLabel')"
            :page-size-options="[12, 24, 48]"
          />
        </div>

        <AppRegistryInspector
          v-model="inspectorOpen"
          :title="inspected?.name ?? ''"
          :eyebrow="inspected ? `WEBHOOK / ${hostOf(inspected.url)}` : undefined"
        >
          <div v-if="inspected" class="cx-inspect">
            <div class="cx-inspect__lead">
              <span class="cap-card__status">
                <i class="cap-dot" :class="inspected.enabled ? 'cap-dot--ok' : 'cap-dot--off'" aria-hidden="true" />
                {{ inspected.enabled ? t('connect.common.enabled') : t('connect.common.disabled') }}
              </span>
            </div>
            <dl class="cx-facts">
              <div>
                <dt>{{ t('connect.webhooks.facts.url') }}</dt>
                <dd class="cx-num">{{ inspected.url }}</dd>
              </div>
              <div>
                <dt>{{ t('connect.webhooks.facts.signing') }}</dt>
                <dd>
                  {{ inspected.secret ? t('connect.webhooks.facts.signed') : t('connect.webhooks.facts.unsigned') }}
                </dd>
              </div>
              <div>
                <dt>{{ t('connect.webhooks.facts.headers') }}</dt>
                <dd class="cx-num">
                  {{ Object.keys(inspected.headers).join(', ') || t('connect.webhooks.facts.noHeaders') }}
                </dd>
              </div>
              <div>
                <dt>{{ t('connect.common.createdAt') }}</dt>
                <dd class="cx-num">{{ formatTime(inspected.created_at) }}</dd>
              </div>
              <div>
                <dt>{{ t('connect.common.updatedAt') }}</dt>
                <dd class="cx-num">{{ formatTime(inspected.updated_at) }}</dd>
              </div>
            </dl>

            <section class="cx-inspect__section">
              <h3 class="cx-inspect__section-title">{{ t('connect.webhooks.facts.events') }}</h3>
              <ul class="cap-card__tags" role="list">
                <li v-for="evt in parseWebhookEvents(inspected.event_types_json)" :key="evt" class="cap-tag">
                  {{ eventLabel(evt) }}
                </li>
              </ul>
            </section>

            <section class="cx-inspect__section" aria-live="polite">
              <h3 class="cx-inspect__section-title">{{ t('connect.webhooks.testTitle') }}</h3>
              <p class="cx-inspect__empty">{{ t('connect.webhooks.testHint') }}</p>
              <p
                v-if="lastTest[inspected.id]"
                class="cap-note"
                :class="{ 'cap-note--info': lastTest[inspected.id].success }"
              >
                <q-icon :name="lastTest[inspected.id].success ? 'check_circle' : 'error'" size="16px" />
                <span class="cap-note__body">{{ testText(lastTest[inspected.id]) }}</span>
              </p>
              <p v-else class="cx-inspect__empty">{{ t('connect.webhooks.testNever') }}</p>
            </section>
          </div>
          <template v-if="inspected" #footer>
            <div class="cx-inspect__actions">
              <q-btn
                unelevated
                no-caps
                color="primary"
                icon="send"
                :label="t('connect.webhooks.testRun')"
                :loading="testingId === inspected.id"
                @click="testRow(inspected)"
              />
              <q-btn flat no-caps icon="edit" :label="t('connect.common.edit')" @click="openEdit(inspected)" />
              <q-btn
                flat
                no-caps
                color="negative"
                icon="delete"
                :label="t('connect.common.remove')"
                @click="confirmDelete(inspected)"
              />
            </div>
          </template>
        </AppRegistryInspector>
      </div>
    </template>

    <WebhookDialog
      ref="dialogRef"
      v-model:open="editorOpen"
      :editing-id="editingId"
      :saving="saving"
      @save="saveWebhook"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import AppRegistryInspector from '../components/layout/AppRegistryInspector.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import CapCard from '../components/capability/CapCard.vue';
import CapStats from '../components/capability/CapStats.vue';
import type { CapFact, CapStatus, CapTag } from '../components/capability/capTypes';
import ConnectGuide from '../components/connect/ConnectGuide.vue';
import WebhookDialog from '../components/webhooks/WebhookDialog.vue';
import { WEBHOOK_EVENT_TYPES, type WebhookRow } from '../features/webhooks/types';
import type { WebhookTestResult } from '../features/webhooks/api';
import { parseWebhookEvents, useWebhooksPage } from '../features/webhooks/useWebhooksPage';
import { formatConnectTime as formatTime, hostOf } from '../features/connect/format';

const {
  t,
  rows,
  loaded,
  loading,
  saving,
  error,
  forbidden,
  search,
  stateFilter,
  stats,
  editorOpen,
  editingId,
  busyId,
  testingId,
  dialogRef,
  page,
  pageSize,
  total,
  pagedRows,
  pageMax,
  inspected,
  inspectorOpen,
  lastTest,
  eventLabel,
  loadRows,
  openCreate,
  openEdit,
  inspect,
  saveWebhook,
  toggleEnabled,
  testRow,
  confirmDelete,
} = useWebhooksPage();

const statusOn = computed<CapStatus>(() => ({ tone: 'ok', label: t('connect.common.enabled') }));
const statusOff = computed<CapStatus>(() => ({ tone: 'off', label: t('connect.common.disabled') }));

function testText(res: WebhookTestResult): string {
  return res.success
    ? t('connect.webhooks.testOk', { status: res.status_code, duration: res.duration_ms })
    : t('connect.webhooks.testFail', { error: res.error || `HTTP ${res.status_code}` });
}

function cardFacts(row: WebhookRow): CapFact[] {
  const test = lastTest.value[row.id];
  return [
    { label: t('connect.webhooks.facts.events'), value: parseWebhookEvents(row.event_types_json).length },
    {
      label: t('connect.webhooks.facts.signing'),
      value: row.secret ? t('connect.webhooks.facts.signed') : t('connect.webhooks.facts.unsigned'),
      tone: row.secret ? undefined : 'watch',
    },
    {
      label: t('connect.webhooks.testTitle'),
      value: test
        ? test.success
          ? `HTTP ${test.status_code}`
          : t('connect.common.abnormal')
        : t('connect.common.none'),
      tone: test ? (test.success ? 'ok' : 'alert') : undefined,
    },
  ];
}

function cardTags(row: WebhookRow): CapTag[] {
  const events = parseWebhookEvents(row.event_types_json);
  const tags: CapTag[] = events.slice(0, 3).map((evt) => ({ label: eventLabel(evt) }));
  if (events.length > 3) tags.push({ label: `+${events.length - 3}` });
  return tags;
}
</script>
