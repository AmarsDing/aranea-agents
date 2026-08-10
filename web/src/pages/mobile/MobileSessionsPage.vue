<template>
  <q-page class="mobile-sessions-page column no-wrap">
    <div v-if="!workspace.coreReady" class="col flex flex-center">
      <q-spinner-dots size="40px" color="accent" />
    </div>

    <template v-else>
      <div class="row items-center justify-between q-px-md q-py-sm mobile-sessions-page__header">
        <div class="text-subtitle1 text-weight-bold">{{ t('mobile.sessionsTitle') }}</div>
        <q-btn
          flat
          round
          dense
          icon="add_comment"
          :aria-label="t('mobile.newSession')"
          color="primary"
          @click="onNewSession"
        >
          <q-tooltip>{{ t('mobile.newSession') }}</q-tooltip>
        </q-btn>
      </div>

      <q-scroll-area class="col">
        <q-list separator>
          <q-item
            v-for="session in sortedSessions"
            :key="session.id"
            v-ripple
            clickable
            class="mobile-session-item"
            @click="onOpenSession(session)"
          >
            <q-item-section>
              <q-item-label lines="1" class="text-body1">{{ session.title || t('chat.untitledSession') }}</q-item-label>
              <q-item-label caption lines="1">
                {{ formatSessionTime(session) }}
                <template v-if="session.message_count"
                  >· {{ t('mobile.messageCount', { n: session.message_count }) }}</template
                >
              </q-item-label>
            </q-item-section>
            <q-item-section v-if="session.context_used_ratio > 0" side class="mobile-session-item__ctx">
              <q-linear-progress
                :value="session.context_used_ratio"
                :color="contextColor(session)"
                track-color="grey-4"
                rounded
                size="6px"
                class="mobile-session-item__ctx-bar"
              />
              <div class="text-caption text-grey text-right">{{ Math.round(session.context_used_ratio * 100) }}%</div>
            </q-item-section>
            <q-item-section side>
              <q-icon name="chevron_right" color="grey-5" />
            </q-item-section>
          </q-item>
        </q-list>

        <div v-if="sortedSessions.length === 0" class="column items-center q-pa-xl text-grey">
          <q-icon name="forum" size="48px" class="q-mb-md" />
          <div class="text-body2">{{ t('mobile.sessionsEmpty') }}</div>
        </div>
      </q-scroll-area>
    </template>
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { storeToRefs } from 'pinia';
import { injectChatWorkspace } from '../../features/chat/composables/chatWorkspaceInjection';
import { sortSessionsForDisplay } from '../../features/session/sessionSort';
import { composerContextColor } from '../../features/chat/composerUsageMetrics';
import { useOfflineSessionList } from '../../features/mobile/useOfflineSessionList';
import { useChatSessionStore } from '../../stores/chat/sessionStore';
import type { SessionView } from '../../components/chat/types';

const { t } = useI18n();
const router = useRouter();
const workspace = injectChatWorkspace();

// P3.2c: offline fallback. The session store only assigns its list after a
// successful load, so when the app opens without connectivity the list would
// be empty; useOfflineSessionList serves the last cached list instead (the
// layout banner flags the stale state).
const sessionStore = useChatSessionStore();
const { error: sessionLoadError } = storeToRefs(sessionStore);
const liveSessions = computed(() => sortSessionsForDisplay(workspace.session.displaySessions));
const agentId = computed(() => workspace.entity.store.selectedAgent?.id?.trim() ?? '');
const { displaySessions: sortedSessions } = useOfflineSessionList({
  live: liveSessions,
  agentId,
  loadError: sessionLoadError,
});

function contextColor(session: SessionView): string {
  return composerContextColor(session.context_status, session.context_used_ratio ?? 0);
}

/** Today → HH:mm; older → MM-DD. Mirrors the desktop sidebar's compact time. */
function formatSessionTime(session: SessionView): string {
  const raw = session.timeline_at || session.at;
  const value = raw ? new Date(raw).getTime() : 0;
  if (!Number.isFinite(value) || value <= 0) return '—';
  const date = new Date(value);
  const now = new Date();
  const sameDay =
    date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth() && date.getDate() === now.getDate();
  if (sameDay) {
    return new Intl.DateTimeFormat([], { hour: '2-digit', minute: '2-digit' }).format(date);
  }
  return new Intl.DateTimeFormat([], { month: '2-digit', day: '2-digit' }).format(date);
}

/**
 * Navigate to the chat detail. The workspace's existing route.query watcher
 * (useChatWorkspace) focuses the session in the store — do NOT call
 * session.onSelectSession here: it route-syncs to the DESKTOP 'chat' route
 * which the breakpoint guard would bounce back.
 */
function onOpenSession(session: SessionView) {
  const agentId = session.agent_id?.trim() || workspace.entity.store.selectedAgent?.id?.trim() || '';
  void router.push({
    name: 'mobile-chat',
    query: agentId ? { session: session.id, agent: agentId } : { session: session.id },
  });
}

async function onNewSession() {
  await workspace.session.onNewSession();
  const created = workspace.session.selectedSessionForUi;
  if (created) {
    onOpenSession(created);
  }
}
</script>

<style scoped>
.mobile-sessions-page__header {
  border-bottom: 1px solid rgb(0 0 0 / 8%);
}

.mobile-session-item {
  min-height: 64px;
}

.mobile-session-item__ctx {
  min-width: 64px;
}

.mobile-session-item__ctx-bar {
  width: 56px;
}
</style>
