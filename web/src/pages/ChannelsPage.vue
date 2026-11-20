<template>
  <!-- Harness › 连接 › 通道：关键数（在线 / 异常 / 未连上 / 停用）→ 筛选 → 构件卡 + 检视面板；还没有通道时按平台开始 -->
  <q-page class="app-standard-page cap-page channels-page">
    <Teleport defer to="#app-tab-actions">
      <q-btn
        v-if="rows.length"
        unelevated
        no-caps
        color="primary"
        icon="add"
        :label="t('connect.channels.create')"
        @click="openCreate()"
      />
    </Teleport>

    <AppStatePanel v-if="forbidden" variant="forbidden" />
    <AppStatePanel v-else-if="!loaded && loading" variant="loading" :rows="4" />
    <AppStatePanel
      v-else-if="error && !rows.length"
      variant="error"
      :description="error"
      :action-label="t('channelsPage.retry')"
      action-icon="refresh"
      @action="loadAll"
    />

    <ConnectGuide
      v-else-if="!rows.length"
      icon="forum"
      :title="t('connect.channels.guideTitle')"
      :description="t('connect.channels.guideDesc')"
      :steps="[t('connect.channels.guideStep1'), t('connect.channels.guideStep2'), t('connect.channels.guideStep3')]"
    >
      <div class="cx-picks">
        <span class="cx-picks__group-title">{{ t('connect.channels.guidePick') }}</span>
        <div v-for="group in catalogGroups" :key="group.name" class="cx-picks__group">
          <span class="cx-picks__group-title">{{ group.name }}</span>
          <div class="cx-picks__grid">
            <button
              v-for="item in group.items"
              :key="item.type"
              type="button"
              class="cx-pick"
              @click="openCreate(item.type)"
            >
              <ChannelPlatformAvatar :type="item.type" :label="item.label" size="30px" />
              <span class="cx-pick__text">
                <span class="cx-pick__name">{{ item.label }}</span>
                <span class="cx-pick__sub">{{ item.receive_modes.join(' / ') }}</span>
              </span>
            </button>
          </div>
        </div>
      </div>
    </ConnectGuide>

    <template v-else>
      <AppStatePanel
        v-if="error"
        variant="partial"
        dense
        :description="error"
        :action-label="t('channelsPage.retry')"
        action-icon="refresh"
        @action="loadAll"
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
          :placeholder="t('connect.channels.search')"
          :aria-label="t('connect.channels.search')"
        >
          <template #prepend><q-icon name="search" /></template>
        </q-input>
        <q-select
          v-model="typeFilter"
          dense
          outlined
          emit-value
          map-options
          :options="typeOptions"
          :aria-label="t('connect.channels.facts.platform')"
        />
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
          @click="loadAll"
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
              icon="forum"
              :code="row.key"
              :title="row.name"
              :description="channelMetadata(row).last_error_message || row.description"
              :status="statusOf(row)"
              :facts="cardFacts(row)"
              :tags="isChannelShared(row) ? [{ label: t('connect.channels.shared') }] : []"
              :selected="inspected?.id === row.id"
              @open="inspect(row)"
            >
              <template #switch>
                <q-toggle
                  :model-value="row.enabled"
                  dense
                  :disable="togglingId === row.id || isChannelShared(row)"
                  :aria-label="row.enabled ? t('connect.common.disable') : t('connect.common.enable')"
                  @update:model-value="toggleRow(row, Boolean($event))"
                />
              </template>
              <template #relation>{{ answeredByText(row) }}</template>
              <template #actions>
                <q-btn
                  flat
                  dense
                  no-caps
                  icon="science"
                  :label="t('connect.channels.test')"
                  :loading="testingId === row.id"
                  :disable="isChannelShared(row)"
                  @click="testRow(row)"
                />
                <q-btn flat dense round icon="more_horiz" :aria-label="t('connect.common.more')">
                  <q-menu anchor="bottom right" self="top right">
                    <q-list dense>
                      <q-item
                        v-if="channelSupportsWebhook(row, catalog)"
                        v-close-popup
                        clickable
                        @click="copyWebhook(row)"
                      >
                        <q-item-section>{{ t('channelsPage.copyWebhook') }}</q-item-section>
                      </q-item>
                      <q-item v-close-popup clickable :disable="isChannelShared(row)" @click="openEdit(row)">
                        <q-item-section>{{ t('connect.common.edit') }}</q-item-section>
                      </q-item>
                      <q-item v-close-popup clickable :disable="isChannelShared(row)" @click="confirmDelete(row)">
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
            :label="t('channelsPage.paginationUnit')"
            :page-size-options="[12, 24, 48]"
          />
        </div>

        <AppRegistryInspector
          v-model="inspectorOpen"
          :title="inspected?.name ?? ''"
          :eyebrow="inspected ? `CHANNEL / ${inspected.key}` : undefined"
        >
          <div v-if="inspected" class="cx-inspect">
            <div class="cx-inspect__lead">
              <ChannelPlatformAvatar
                :type="channelType(inspected)"
                :label="inspected.name"
                :metadata="channelMetadata(inspected)"
                size="28px"
              />
              <span class="cap-card__status">
                <i class="cap-dot" :class="`cap-dot--${statusOf(inspected).tone}`" aria-hidden="true" />
                {{ statusOf(inspected).label }}
              </span>
              <span v-if="isChannelShared(inspected)" class="cap-tag">{{ t('connect.channels.shared') }}</span>
            </div>
            <p v-if="channelMetadata(inspected).last_error_message" class="cap-note">
              <q-icon name="error_outline" size="16px" />
              <span class="cap-note__body">
                <b>{{ t('connect.channels.facts.lastError') }}</b> · {{ channelMetadata(inspected).last_error_message }}
              </span>
            </p>
            <dl class="cx-facts">
              <div>
                <dt>{{ t('connect.channels.facts.answeredBy') }}</dt>
                <dd>{{ answeredByText(inspected) }}</dd>
              </div>
              <div>
                <dt>{{ t('connect.channels.facts.platform') }}</dt>
                <dd>{{ platformLabel(inspected) }}</dd>
              </div>
              <div>
                <dt>{{ t('connect.channels.facts.mode') }}</dt>
                <dd class="cx-num">{{ receiveMode(inspected) }}</dd>
              </div>
              <div>
                <dt>{{ t('connect.channels.facts.key') }}</dt>
                <dd class="cx-num">{{ inspected.key }}</dd>
              </div>
              <div v-if="channelExternalID(inspected) !== '—'">
                <dt>{{ t('connect.channels.facts.externalId') }}</dt>
                <dd class="cx-num">{{ channelExternalID(inspected) }}</dd>
              </div>
              <div v-if="channelMetadata(inspected).runtime_connected_since">
                <dt>{{ t('connect.channels.facts.connectedSince') }}</dt>
                <dd class="cx-num">{{ formatTime(channelMetadata(inspected).runtime_connected_since) }}</dd>
              </div>
              <div>
                <dt>{{ t('connect.common.updatedAt') }}</dt>
                <dd class="cx-num">{{ formatTime(inspected.updated_at) }}</dd>
              </div>
            </dl>

            <section v-if="channelSupportsWebhook(inspected, catalog)" class="cx-inspect__section">
              <h3 class="cx-inspect__section-title">
                {{ t('connect.channels.facts.webhookUrl') }}
                <q-btn
                  flat
                  dense
                  no-caps
                  icon="content_copy"
                  :label="t('connect.common.copy')"
                  @click="copyWebhook(inspected)"
                />
              </h3>
              <div class="cx-code">{{ channelWebhookURL(inspected) }}</div>
            </section>

            <p v-if="!supportsTest(inspected)" class="cap-note cap-note--info">
              <q-icon name="info_outline" size="16px" />
              <span class="cap-note__body">{{ t('connect.channels.testStaticHint') }}</span>
            </p>

            <section class="cx-inspect__section">
              <h3 class="cx-inspect__section-title">
                {{ t('connect.channels.activityTitle') }}
                <q-btn
                  flat
                  dense
                  no-caps
                  icon="open_in_full"
                  :label="t('connect.channels.activityOpen')"
                  @click="opsOpen = true"
                />
              </h3>
              <p class="cx-inspect__empty">{{ t('connect.channels.activityHint') }}</p>
            </section>
          </div>
          <template v-if="inspected && !isChannelShared(inspected)" #footer>
            <div class="cx-inspect__actions">
              <q-btn
                unelevated
                no-caps
                color="primary"
                icon="science"
                :label="t('connect.channels.test')"
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

    <AppDialog
      v-model="opsOpen"
      size="xl"
      :title="t('connect.channels.activityTitle')"
      :subtitle="inspected ? `${inspected.name} · ${inspected.key}` : ''"
    >
      <div v-if="inspected" class="channel-ops-grid">
        <ChannelTurnJobsPanel :channel-id="inspected.id" />
        <ChannelDeliveriesPanel :channel-id="inspected.id" />
      </div>
    </AppDialog>

    <ChannelEditorDialog
      v-model="editorOpen"
      :catalog="catalog"
      :row="editingRow"
      :credentials="editingCredentials"
      :initial-type="createType"
      @saved="onSaved"
      @tested="loadAll"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import AppDialog from '../components/common/AppDialog.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import AppRegistryInspector from '../components/layout/AppRegistryInspector.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import CapCard from '../components/capability/CapCard.vue';
import CapStats from '../components/capability/CapStats.vue';
import type { CapFact, CapStatus } from '../components/capability/capTypes';
import ChannelPlatformAvatar from '../components/channels/ChannelPlatformAvatar.vue';
import ConnectGuide from '../components/connect/ConnectGuide.vue';
import {
  channelExternalID,
  channelMetadata,
  channelSupportsWebhook,
  channelType,
  channelWebhookURL,
  isChannelShared,
  receiveMode,
} from '../components/channels/channelUi';
import ChannelEditorDialog from '../features/channels/ChannelEditorDialog.vue';
import ChannelDeliveriesPanel from '../features/channels/ChannelDeliveriesPanel.vue';
import ChannelTurnJobsPanel from '../features/channels/ChannelTurnJobsPanel.vue';
import type { ChannelRow } from '../features/channels/types';
import { CHANNEL_HEALTH_TONE, channelHealth, useChannelsPage } from '../features/channels/useChannelsPage';
import { formatConnectTime as formatTime } from '../features/connect/format';

const {
  t,
  rows,
  catalog,
  loaded,
  loading,
  error,
  forbidden,
  search,
  typeFilter,
  stateFilter,
  typeOptions,
  healthCounts,
  page,
  pageSize,
  pageMax,
  total,
  pagedRows,
  togglingId,
  testingId,
  editorOpen,
  editingRow,
  editingCredentials,
  createType,
  inspected,
  inspectorOpen,
  platformLabel,
  supportsTest,
  answeredBy,
  loadAll,
  openCreate,
  openEdit,
  inspect,
  onSaved,
  toggleRow,
  testRow,
  copyWebhook,
  confirmDelete,
} = useChannelsPage();

const opsOpen = ref(false);

const healthLabel = computed(() => ({
  online: t('connect.channels.online'),
  alert: t('connect.common.abnormal'),
  pending: t('connect.channels.pending'),
  off: t('connect.common.disabled'),
}));

const stats = computed(() => [
  { key: '', label: t('connect.common.all'), value: rows.value.length, clickable: true },
  ...(['online', 'alert', 'pending', 'off'] as const).map((key) => ({
    key,
    label: healthLabel.value[key],
    value: healthCounts.value[key],
    tone: CHANNEL_HEALTH_TONE[key],
    clickable: true,
  })),
]);

const catalogGroups = computed(() => {
  const groups = new Map<string, typeof catalog.value>();
  for (const item of [...catalog.value].sort((a, b) => a.sort_order - b.sort_order)) {
    const name = item.group || '—';
    groups.set(name, [...(groups.get(name) ?? []), item]);
  }
  return [...groups.entries()].map(([name, items]) => ({ name, items }));
});

function statusOf(row: ChannelRow): CapStatus {
  const health = channelHealth(row);
  return { tone: CHANNEL_HEALTH_TONE[health], label: healthLabel.value[health] };
}

function answeredByText(row: ChannelRow): string {
  const target = answeredBy(row);
  const base = target.label || t('connect.channels.facts.notRouted');
  return target.rules ? `${base} · ${t('connect.channels.rulesMore', { count: target.rules })}` : base;
}

function cardFacts(row: ChannelRow): CapFact[] {
  const target = answeredBy(row);
  return [
    { label: t('connect.channels.facts.platform'), value: platformLabel(row) },
    { label: t('connect.channels.facts.mode'), value: receiveMode(row) },
    {
      label: t('connect.channels.facts.answeredBy'),
      value: target.label ? target.label.split(' · ').pop() || target.label : t('connect.channels.facts.notRouted'),
      tone: target.label ? undefined : 'watch',
    },
  ];
}
</script>

<style scoped>
.channel-ops-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--nt-space-4);
}

@media (width <= 1100px) {
  .channel-ops-grid {
    grid-template-columns: 1fr;
  }
}
</style>
