<template>
  <q-page class="app-standard-page session-detail-page sessions-page">
    <div v-if="loadingSession" class="column items-center justify-center" style="min-height: 300px">
      <q-spinner color="primary" size="40px" />
      <div class="q-mt-md text-grey-7">加载中...</div>
    </div>

    <template v-else-if="sessionError">
      <q-banner class="bg-negative text-white rounded-borders q-mb-md">
        <template #avatar><q-icon name="error" /></template>
        {{ sessionError }}
        <template #action>
          <q-btn flat label="返回列表" @click="router.push({ name: 'sessions' })" />
        </template>
      </q-banner>
    </template>

    <template v-else-if="session">
      <div class="row items-center q-mb-md">
        <q-btn flat round icon="arrow_back" class="q-mr-sm" @click="router.push({ name: 'sessions' })" />
        <div class="col">
          <div class="row items-center q-gutter-sm">
            <div class="text-h5" style="color: var(--color-text-primary)">{{ session.title || '未命名会话' }}</div>
            <q-chip dense :color="ownerChipColor(session.owner_type)" text-color="white">{{
              ownerLabel(session.owner_type)
            }}</q-chip>
            <SessionStatusBadge
              :status="session.status"
              :status-reason="session.status_reason"
              :status-changed-at="session.status_changed_at"
            />
          </div>
          <div class="text-caption text-grey-7 q-mt-xs">
            {{ session.id }} · 创建 {{ formatSessionDate(session.created_at) }} · 最后活跃
            {{ formatSessionDate(session.last_message_at || session.updated_at) }}
          </div>
        </div>
        <div class="row q-gutter-sm">
          <q-btn-dropdown
            outline
            rounded
            icon="download"
            label="导出"
            class="sessions-btn-accent-outline"
            :loading="exporting"
          >
            <q-list dense>
              <q-item v-close-popup clickable @click="handleExport('markdown')">
                <q-item-section avatar><q-icon name="description" /></q-item-section>
                <q-item-section>Markdown</q-item-section>
              </q-item>
              <q-item v-close-popup clickable @click="handleExport('json')">
                <q-item-section avatar><q-icon name="data_object" /></q-item-section>
                <q-item-section>JSON</q-item-section>
              </q-item>
            </q-list>
          </q-btn-dropdown>
          <q-btn
            v-if="!!session.archived_at"
            outline
            rounded
            icon="restore"
            label="恢复"
            class="sessions-btn-accent-outline"
            @click="handleRestore"
          />
          <q-btn
            outline
            rounded
            icon="chat"
            label="继续会话"
            class="sessions-btn-accent-outline"
            :to="{ name: 'chat', query: { session: session.id } }"
          />
          <q-btn
            flat
            rounded
            icon="archive"
            label="归档"
            class="sessions-btn-ghost"
            :disable="!!session.archived_at"
            @click="handleArchive"
          />
        </div>
      </div>

      <q-card flat class="q-mb-md">
        <q-card-section class="row q-col-gutter-md">
          <div class="col-12 col-md-4">
            <div class="text-caption text-grey-7">Context</div>
            <q-linear-progress
              rounded
              size="12px"
              :value="ratioValue(session.context_used_ratio)"
              :color="contextProgressColor(session.context_status)"
              class="q-mt-sm"
            />
            <div class="text-caption text-grey-7 q-mt-xs">
              当前 {{ formatPercent(session.context_used_ratio) }} · 最高
              {{ formatPercent(session.max_context_used_ratio) }}
            </div>
          </div>
          <div class="col-6 col-md-2">
            <div class="text-caption text-grey-7">消息</div>
            <div class="text-h6" style="color: var(--color-text-primary)">{{ session.message_count }}</div>
          </div>
          <div class="col-6 col-md-2">
            <div class="text-caption text-grey-7">模型调用</div>
            <div class="text-h6" style="color: var(--color-text-primary)">{{ session.model_call_count }}</div>
          </div>
          <div class="col-6 col-md-2">
            <div class="text-caption text-grey-7">Token</div>
            <div class="text-h6" style="color: var(--color-text-primary)">{{ formatNumber(session.total_tokens) }}</div>
          </div>
          <div class="col-6 col-md-2">
            <div class="text-caption text-grey-7">费用</div>
            <div class="text-h6" style="color: var(--color-text-primary)">
              {{ formatCostMicroUsd(session.total_cost_micro_usd) }}
            </div>
          </div>
        </q-card-section>
      </q-card>

      <q-tabs
        v-model="activeTab"
        dense
        class="text-grey-7"
        active-color="primary"
        indicator-color="primary"
        align="left"
        narrow-indicator
      >
        <q-tab name="turns" icon="sync_alt" label="Turns" />
        <q-tab name="runs" icon="play_circle" label="Runs" />
        <q-tab v-if="showParticipants" name="participants" icon="groups" label="Participants" />
        <q-tab name="messages" icon="chat" label="Messages" />
        <q-tab name="timeline" icon="timeline" label="Timeline" />
      </q-tabs>

      <q-separator />

      <q-tab-panels v-model="activeTab" animated class="bg-transparent">
        <q-tab-panel name="turns" class="q-pa-none q-mt-md">
          <SessionTurnsPanel :session-id="session.id" />
        </q-tab-panel>

        <q-tab-panel name="runs" class="q-pa-none q-mt-md">
          <SessionRunsPanel :session-id="session.id" />
        </q-tab-panel>

        <q-tab-panel v-if="showParticipants" name="participants" class="q-pa-none q-mt-md">
          <SessionParticipantsPanel :session-id="session.id" />
        </q-tab-panel>

        <q-tab-panel name="messages" class="q-pa-none q-mt-md">
          <SessionMessagesPanel :session-id="session.id" />
        </q-tab-panel>

        <q-tab-panel name="timeline" class="q-pa-none q-mt-md">
          <SessionTimelinePanel
            :session-id="session.id"
            :session-title="session.title || '未命名会话'"
            :focus-tool-id="focusToolId"
            :timeline="timelinePanel.timeline.value"
            :loading="timelinePanel.loading.value"
            :error="timelinePanel.error.value"
            :kind-filter="timelinePanel.kindFilter.value"
            :sort-order="timelinePanel.sortOrder.value"
            :stats="timelinePanel.stats.value"
            :offset="timelinePanel.offset.value"
            :total="timelinePanel.total.value"
            :page-size="timelinePanel.pageSize"
            :page-label="timelinePanel.pageLabel.value"
            @refresh="timelinePanel.loadTimeline"
            @update:kind-filter="timelinePanel.kindFilter.value = $event"
            @update:sort-order="timelinePanel.sortOrder.value = $event"
            @prev-page="timelinePanel.prevPage"
            @next-page="timelinePanel.nextPage"
          />
        </q-tab-panel>
      </q-tab-panels>
    </template>
  </q-page>
</template>

<script setup lang="ts">
import {
  contextProgressColor,
  formatCostMicroUsd,
  formatNumber,
  formatPercent,
  formatSessionDate,
  ownerChipColor,
  ownerLabel,
  ratioValue,
} from '../components/sessions/sessionUi';
import SessionStatusBadge from '../components/sessions/SessionStatusBadge.vue';
import SessionTurnsPanel from '../components/sessions/SessionTurnsPanel.vue';
import SessionRunsPanel from '../components/sessions/SessionRunsPanel.vue';
import SessionParticipantsPanel from '../components/sessions/SessionParticipantsPanel.vue';
import SessionMessagesPanel from '../components/sessions/SessionMessagesPanel.vue';
import SessionTimelinePanel from '../components/sessions/SessionTimelinePanel.vue';
import { useSessionDetailPage } from '../features/session/useSessionDetailPage';

const {
  router,
  session,
  loadingSession,
  sessionError,
  activeTab,
  focusToolId,
  showParticipants,
  exporting,
  handleArchive,
  handleRestore,
  handleExport,
  timelinePanel,
} = useSessionDetailPage();
</script>
