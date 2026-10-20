<template>
  <div class="kb-glass kb-topbar">
    <!-- Vault 切换 -->
    <q-btn-dropdown
      flat
      no-caps
      dense
      class="kb-topbar__vault"
      :label="currentVaultName"
      icon="inventory_2"
      :title="t('knowledgePage.workbench.vaultSwitcher')"
    >
      <q-list dense class="kb-topbar__vault-list">
        <q-item
          v-for="c in collections"
          :key="c.id"
          v-close-popup
          clickable
          :active="c.id === currentVaultId"
          @click="$emit('switch-vault', c.id)"
        >
          <q-item-section avatar>
            <q-icon :name="c.vault_backend === 'team' ? 'groups' : 'inventory_2'" size="18px" />
          </q-item-section>
          <q-item-section>{{ c.name || c.id }}</q-item-section>
          <q-item-section side>
            <span class="kb-topbar__vault-count">{{ c.document_count }}</span>
          </q-item-section>
        </q-item>
      </q-list>
    </q-btn-dropdown>

    <div class="kb-topbar__spacer" />

    <!-- 快速动作 -->
    <q-btn
      flat
      dense
      no-caps
      icon="search"
      :label="t('knowledgePage.workbench.searchTitle')"
      class="kb-topbar__action"
      @click="$emit('open-search')"
    >
      <q-tooltip>Ctrl+Shift+F</q-tooltip>
    </q-btn>
    <q-btn
      flat
      dense
      no-caps
      icon="bolt"
      :label="t('knowledgePage.workbench.quickSwitcher')"
      class="kb-topbar__action"
      @click="$emit('open-quick-switcher')"
    >
      <q-tooltip>Ctrl+O</q-tooltip>
    </q-btn>
    <q-btn
      flat
      dense
      no-caps
      icon="terminal"
      :label="t('knowledgePage.workbench.commandPalette')"
      class="kb-topbar__action"
      @click="$emit('open-command-palette')"
    >
      <q-tooltip>Ctrl+K</q-tooltip>
    </q-btn>
    <!-- SP3-P3 审查待办徽章：治理提案 + 写回待审（60s 轮询 + 聚焦刷新，失败静默降级） -->
    <q-btn
      flat
      dense
      round
      icon="notifications"
      :title="t('knowledgePage.workbench.reviewBadge.title')"
      class="kb-topbar__icon"
    >
      <q-badge v-if="reviewTotal > 0" color="negative" floating :label="reviewTotal > 99 ? '99+' : reviewTotal" />
      <q-tooltip>
        <div>
          {{ t('knowledgePage.workbench.reviewBadge.governance') }}:
          {{ formatReviewCount(reviewCounts.governancePending) }}
        </div>
        <div>
          {{ t('knowledgePage.workbench.reviewBadge.writeback') }}:
          {{ formatReviewCount(reviewCounts.writebackPending) }}
        </div>
        <div>{{ t('knowledgePage.workbench.reviewBadge.lastRefresh', { time: reviewLastRefreshText }) }}</div>
      </q-tooltip>
      <q-menu>
        <q-list dense class="kb-topbar__vault-list kb-topbar__review-list">
          <q-item v-close-popup clickable @click="$emit('open-review-governance')">
            <q-item-section avatar>
              <q-icon name="gavel" size="18px" />
            </q-item-section>
            <q-item-section>{{ t('knowledgePage.workbench.reviewBadge.governance') }}</q-item-section>
            <q-item-section side>
              <span class="kb-topbar__review-count">{{ formatReviewCount(reviewCounts.governancePending) }}</span>
            </q-item-section>
          </q-item>
          <q-item v-close-popup clickable @click="$emit('open-review-writeback')">
            <q-item-section avatar>
              <q-icon name="rate_review" size="18px" />
            </q-item-section>
            <q-item-section>{{ t('knowledgePage.workbench.reviewBadge.writeback') }}</q-item-section>
            <q-item-section side>
              <span class="kb-topbar__review-count">{{ formatReviewCount(reviewCounts.writebackPending) }}</span>
            </q-item-section>
          </q-item>
          <q-separator />
          <q-item v-close-popup clickable @click="refreshReviewBadge()">
            <q-item-section avatar>
              <q-icon name="refresh" size="18px" />
            </q-item-section>
            <q-item-section>{{ t('knowledgePage.workbench.reviewBadge.refresh') }}</q-item-section>
          </q-item>
        </q-list>
      </q-menu>
    </q-btn>
    <q-btn
      flat
      dense
      round
      icon="science"
      :title="tt('hitTest')"
      class="kb-topbar__icon"
      @click="$emit('open-hit-test')"
    />
    <q-btn
      flat
      dense
      round
      icon="hub"
      :title="t('knowledgePage.workbench.openGraph')"
      class="kb-topbar__icon"
      @click="$emit('open-graph')"
    />
    <q-btn
      flat
      dense
      round
      icon="tune"
      :title="t('knowledgePage.workbench.openSettings')"
      class="kb-topbar__icon"
      @click="$emit('open-settings')"
    />
  </div>
</template>

<script setup lang="ts">
// SP2 §SP2-1 顶栏：Vault 切换 / ⌘O / ⌘K / 图谱 / 设置浮层入口（纯受控组件）。
import { computed, toRef } from 'vue';
import { useI18n } from 'vue-i18n';
import { useReviewBadge } from '../../../features/knowledge/useReviewBadge';
import type { KnowledgeCollection } from '../../../features/knowledge/types';

const props = defineProps<{
  collections: KnowledgeCollection[];
  currentVaultId: string;
}>();

defineEmits<{
  'switch-vault': [id: string];
  'open-quick-switcher': [];
  'open-command-palette': [];
  'open-search': [];
  'open-graph': [];
  'open-settings': [];
  'open-hit-test': [];
  /** SP3-P3：审查待办菜单入口（由 KnowledgeWorkbench 路由到 useWorkbenchCommands 的对应命令） */
  'open-review-governance': [];
  'open-review-writeback': [];
}>();

const { t } = useI18n();

// G-19 hit-test 入口文案用组件内消息（全局 locale 文件归并行会话维护，不动）。
let tt: (key: string) => string;
try {
  tt = useI18n({
    useScope: 'local',
    inheritLocale: true,
    messages: {
      'zh-CN': { hitTest: '检索排查（hit-test）：同一查询的 dense / sparse / RRF 对比' },
      'en-US': { hitTest: 'Hit-test: compare dense / sparse / RRF for one query' },
    },
  }).t;
} catch {
  tt = (key) => key;
}

const currentVaultName = computed(() => props.collections.find((c) => c.id === props.currentVaultId)?.name ?? '—');

// SP3-P3 审查待办徽章：跟随当前 vault 轮询（挂载/卸载自动 start/stop），静默降级。
const {
  counts: reviewCounts,
  total: reviewTotal,
  lastRefreshAt: reviewLastRefreshAt,
  refresh: refreshReviewBadge,
} = useReviewBadge(toRef(props, 'currentVaultId'));

const reviewLastRefreshText = computed(() =>
  reviewLastRefreshAt.value
    ? new Date(reviewLastRefreshAt.value).toLocaleTimeString()
    : t('knowledgePage.workbench.reviewBadge.never'),
);

function formatReviewCount(n: number | null): string {
  return n === null ? '—' : String(n);
}
</script>

<style lang="sass" scoped>
.kb-topbar
  display: flex
  align-items: center
  gap: 6px
  padding: 6px 12px
  flex: none

  &__vault
    color: var(--kb-text-primary)
    font-weight: 600

  &__vault-list
    min-width: 220px
    background: var(--glass-elevated)
    border: 1px solid var(--glass-border)

  &__vault-count
    font-size: 11px
    color: var(--kb-text-dim)
    font-variant-numeric: tabular-nums

  &__review-list
    min-width: 240px

  &__review-count
    font-size: 12px
    color: var(--kb-text-dim)
    font-variant-numeric: tabular-nums

  &__spacer
    flex: 1

  &__action
    color: var(--kb-text-dim)
    font-size: 12px

    &:hover
      color: var(--kb-accent-cyan)

  &__icon
    color: var(--kb-text-dim)

    &:hover
      color: var(--kb-accent-cyan)

  &__icon--active
    color: var(--kb-accent-cyan)
</style>
