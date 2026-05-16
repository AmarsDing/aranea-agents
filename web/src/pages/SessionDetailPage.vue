<template>
  <q-page class="app-page-cream session-detail-page q-pa-md">
    <div v-if="loadingSession" class="column items-center justify-center" style="min-height: 300px">
      <q-spinner color="primary" size="40px" />
      <div class="q-mt-md text-grey-7">Loading session...</div>
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
            <div class="text-h5" style="color: var(--color-text-primary)">{{ session.title }}</div>
            <q-chip dense :color="ownerChipColor(session.owner_type)" text-color="white">{{ ownerLabel(session.owner_type) }}</q-chip>
            <q-badge :color="statusBadgeColor(session.status)">{{ session.status }}</q-badge>
          </div>
          <div class="text-caption text-grey-7 q-mt-xs">
            {{ session.id }} · 创建 {{ formatDate(session.created_at) }} · 最后活跃 {{ formatDate(session.last_message_at || session.updated_at) }}
          </div>
        </div>
        <div class="row q-gutter-sm">
          <q-btn v-if="session.status === 'archived'" outline rounded icon="restore" label="恢复" class="sessions-btn-accent-outline" @click="handleRestore" />
          <q-btn outline rounded icon="chat" label="继续会话" class="sessions-btn-accent-outline" :to="{ name: 'chat' }" />
          <q-btn flat rounded icon="archive" label="归档" class="sessions-btn-ghost" :disable="session.status === 'archived'" @click="handleArchive" />
        </div>
      </div>

      <q-card flat class="q-mb-md">
        <q-card-section class="row q-col-gutter-md">
          <div class="col-12 col-md-4">
            <div class="text-caption text-grey-7">Context</div>
            <q-linear-progress rounded size="12px" :value="ratioValue(session.context_used_ratio)" :color="contextProgressColor(session.context_status)" class="q-mt-sm" />
            <div class="text-caption text-grey-7 q-mt-xs">
              当前 {{ formatPercent(session.context_used_ratio) }} · 最高 {{ formatPercent(session.max_context_used_ratio) }}
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
            <div class="text-h6" style="color: var(--color-text-primary)">{{ formatCostMicroUsd(session.total_cost_micro_usd) }}</div>
          </div>
        </q-card-section>
      </q-card>

      <q-tabs v-model="activeTab" dense class="text-grey-7" active-color="primary" indicator-color="primary" align="left" narrow-indicator>
        <q-tab name="turns" icon="sync_alt" label="Turns" />
        <q-tab name="messages" icon="chat" label="Messages" />
        <q-tab name="timeline" icon="timeline" label="Timeline" />
      </q-tabs>

      <q-separator />

      <q-tab-panels v-model="activeTab" animated class="bg-transparent">
        <q-tab-panel name="turns" class="q-pa-none q-mt-md">
          <SessionTurnsPanel :session-id="session.id" />
        </q-tab-panel>

        <q-tab-panel name="messages" class="q-pa-none q-mt-md">
          <SessionMessagesPanel :session-id="session.id" />
        </q-tab-panel>

        <q-tab-panel name="timeline" class="q-pa-none q-mt-md">
          <SessionTimelinePanel :session-id="session.id" :session-title="session.title" />
        </q-tab-panel>
      </q-tab-panels>
    </template>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  archiveSession,
  getSession,
  restoreSession,
  type Session
} from "../features/session/api";
import {
  contextProgressColor,
  formatCostMicroUsd,
  formatNumber,
  formatPercent,
  ownerChipColor,
  ownerLabel,
  ratioValue,
  statusBadgeColor
} from "../components/sessions/sessionUi";
import SessionTurnsPanel from "../components/sessions/SessionTurnsPanel.vue";
import SessionMessagesPanel from "../components/sessions/SessionMessagesPanel.vue";
import SessionTimelinePanel from "../components/sessions/SessionTimelinePanel.vue";

const route = useRoute();
const router = useRouter();

const session = ref<Session | null>(null);
const loadingSession = ref(true);
const sessionError = ref("");
const activeTab = ref("turns");

function formatDate(value: string) {
  if (!value) return "—";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

async function loadSession() {
  const id = String(route.params.sessionId || "");
  if (!id) {
    sessionError.value = "Missing session ID";
    loadingSession.value = false;
    return;
  }
  loadingSession.value = true;
  sessionError.value = "";
  try {
    session.value = await getSession(id);
  } catch (err) {
    sessionError.value = err instanceof Error ? err.message : "Failed to load session";
  } finally {
    loadingSession.value = false;
  }
}

async function handleArchive() {
  if (!session.value) return;
  await archiveSession(session.value.id);
  session.value = { ...session.value, status: "archived" };
}

async function handleRestore() {
  if (!session.value) return;
  try {
    session.value = await restoreSession(session.value.id);
  } catch (err) {
    console.error("Restore failed", err);
  }
}

onMounted(loadSession);
</script>

<style scoped>
.session-detail-page {
  min-height: 100%;
}
</style>
