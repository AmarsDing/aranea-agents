<template>
  <transition name="chat-side">
    <aside v-show="open" class="chat-side chat-side--right column no-wrap">
      <div class="chat-session-header row items-center justify-between">
        <div class="text-caption text-cream-muted text-uppercase">
          Session
        </div>
        <q-badge rounded color="primary" :label="sessions.length" />
      </div>
      <q-scroll-area class="col">
        <q-list class="chat-session-list" dense>
          <template v-for="group in timelineGroups" :key="group.key">
            <q-item-label header class="chat-timeline-label">
              {{ group.label }}
            </q-item-label>
            <q-item
              v-for="session in group.sessions"
              :key="session.id"
              clickable
              :active="selectedSessionId === session.id"
              :active-class="isDark ? 'bg-primary' : 'cream-menu-item--active'"
              class="chat-session-item rounded-borders q-mb-sm q-px-sm"
              :class="{ 'chat-session-item--active': selectedSessionId === session.id }"
              @click="$emit('select', session.id)"
            >
              <q-item-section side>
                <q-circular-progress
                  :value="session.context_used_ratio * 100"
                  show-value
                  size="32px"
                  :thickness="0.22"
                  color="primary"
                >
                  <span class="text-caption" style="font-size: 0.6rem">
                    {{ Math.round(session.context_used_ratio * 100) }}%
                  </span>
                </q-circular-progress>
              </q-item-section>

              <q-item-section class="ellipsis" style="max-width: 1px">
                <div class="row items-center no-wrap justify-end q-gutter-xs">
                  <q-icon
                    v-if="isFavorite(session.id)"
                    name="star"
                    color="amber-7"
                    size="14px"
                  />
                  <q-badge
                    class="chat-session-time-badge"
                    rounded
                    :label="shortTime(session)"
                  />
                </div>
                <q-item-label class="chat-session-title ellipsis" lines="2" style="text-align: right">
                  {{ session.title }}
                </q-item-label>
              </q-item-section>

              <q-item-section side class="chat-session-menu-section">
                <q-btn
                  dense
                  round
                  flat
                  size="sm"
                  icon="more_horiz"
                  class="chat-session-menu-btn"
                  :aria-label="t('chat.sessionMore')"
                  @click.stop
                >
                  <q-menu anchor="bottom right" self="top right" class="chat-session-menu">
                    <q-list dense style="min-width: 136px">
                      <q-item clickable v-close-popup @click="renameSession(session)">
                        <q-item-section avatar><q-icon name="edit" size="18px" /></q-item-section>
                        <q-item-section>{{ t("chat.rename") }}</q-item-section>
                      </q-item>
                      <q-item clickable v-close-popup @click="togglePin(session.id)">
                        <q-item-section avatar>
                          <q-icon :name="isPinned(session.id) ? 'push_pin' : 'push_pin'" size="18px" />
                        </q-item-section>
                        <q-item-section>{{ isPinned(session.id) ? t("chat.unpin") : t("chat.pin") }}</q-item-section>
                      </q-item>
                      <q-item clickable v-close-popup @click="toggleFavorite(session.id)">
                        <q-item-section avatar>
                          <q-icon :name="isFavorite(session.id) ? 'star' : 'star_border'" size="18px" />
                        </q-item-section>
                        <q-item-section>{{ isFavorite(session.id) ? t("chat.unfavorite") : t("chat.favorite") }}</q-item-section>
                      </q-item>
                      <q-item clickable v-close-popup @click.stop="openTrace(session.id)">
                        <q-item-section avatar><q-icon name="timeline" size="18px" /></q-item-section>
                        <q-item-section>历史追踪</q-item-section>
                      </q-item>
                      <q-item clickable v-close-popup @click.stop="openDetail(session.id)">
                        <q-item-section avatar><q-icon name="open_in_new" size="18px" /></q-item-section>
                        <q-item-section>详情页</q-item-section>
                      </q-item>
                      <q-item v-if="session.status === 'archived'" clickable v-close-popup @click.stop="$emit('restore', session.id)">
                        <q-item-section avatar><q-icon name="restore" size="18px" /></q-item-section>
                        <q-item-section>恢复会话</q-item-section>
                      </q-item>
                      <q-item v-else clickable v-close-popup @click.stop="$emit('archive', session.id)">
                        <q-item-section avatar><q-icon name="archive" size="18px" /></q-item-section>
                        <q-item-section>归档</q-item-section>
                      </q-item>
                      <q-item clickable v-close-popup class="text-negative" @click="$emit('delete', 'session', session.id)">
                        <q-item-section avatar><q-icon name="delete" size="18px" /></q-item-section>
                        <q-item-section>{{ t("chat.remove") }}</q-item-section>
                      </q-item>
                    </q-list>
                  </q-menu>
                </q-btn>
              </q-item-section>
            </q-item>
          </template>
        </q-list>
      </q-scroll-area>
      <q-separator class="cream-sep" />
      <div class="chat-session-actions row no-wrap q-gutter-sm">
        <q-btn
          unelevated
          dense
          class="chat-primary-btn col"
          color="primary"
          no-caps
          :label="t('chat.newSession')"
          @click="$emit('new-session')"
        />
        <q-btn
          outline
          dense
          class="chat-outline-danger-btn col"
          color="negative"
          no-caps
          :label="t('chat.clearAllSession')"
          @click="$emit('delete', 'all', '')"
        />
      </div>
    </aside>
  </transition>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import type { DeleteKind, SessionView } from "./types";

const props = defineProps<{
  open: boolean;
  sessions: SessionView[];
  selectedSessionId?: string | null;
  isDark: boolean;
}>();

const emit = defineEmits<{
  select: [id: string];
  "new-session": [];
  rename: [payload: { id: string; title: string }];
  delete: [kind: DeleteKind, id: string];
  trace: [id: string];
  restore: [id: string];
  archive: [id: string];
  detail: [id: string];
}>();

const { t } = useI18n();
const $q = useQuasar();

const PINNED_KEY = "chat:pinned-sessions";
const FAVORITE_KEY = "chat:favorite-sessions";

const pinnedIDs = ref(new Set(loadIDs(PINNED_KEY)));
const favoriteIDs = ref(new Set(loadIDs(FAVORITE_KEY)));

const timelineGroups = computed(() => {
  const sorted = [...props.sessions].sort((a, b) => sessionTime(b) - sessionTime(a));
  const pinned = sorted.filter((session) => pinnedIDs.value.has(session.id));
  const regular = sorted.filter((session) => !pinnedIDs.value.has(session.id));
  const groups: Array<{ key: string; label: string; sessions: SessionView[] }> = [];

  if (pinned.length) groups.push({ key: "pinned", label: t("chat.pinnedSessions"), sessions: pinned });
  const buckets = new Map<string, { label: string; sessions: SessionView[] }>();
  for (const session of regular) {
    const bucket = timelineBucket(session);
    if (!buckets.has(bucket.key)) buckets.set(bucket.key, { label: bucket.label, sessions: [] });
    buckets.get(bucket.key)!.sessions.push(session);
  }
  for (const key of ["today", "yesterday", "seven", "thirty", "older"]) {
    const bucket = buckets.get(key);
    if (bucket?.sessions.length) groups.push({ key, ...bucket });
  }
  return groups;
});

function renameSession(session: SessionView) {
  $q.dialog({
    title: t("chat.rename"),
    prompt: {
      model: session.title,
      type: "text",
      isValid: (value) => Boolean(String(value ?? "").trim())
    },
    cancel: true,
    persistent: true
  }).onOk((value) => {
    emit("rename", { id: session.id, title: String(value ?? "").trim() });
  });
}

function openTrace(sessionID: string) {
  emit("trace", sessionID);
}

function openDetail(sessionID: string) {
  emit("detail", sessionID);
}

function isPinned(id: string) {
  return pinnedIDs.value.has(id);
}

function isFavorite(id: string) {
  return favoriteIDs.value.has(id);
}

function togglePin(id: string) {
  pinnedIDs.value = toggleID(pinnedIDs.value, id);
  saveIDs(PINNED_KEY, pinnedIDs.value);
}

function toggleFavorite(id: string) {
  favoriteIDs.value = toggleID(favoriteIDs.value, id);
  saveIDs(FAVORITE_KEY, favoriteIDs.value);
}

function shortTime(session: SessionView) {
  const time = sessionTime(session);
  if (!time) return session.at || "—";
  return new Intl.DateTimeFormat([], { hour: "2-digit", minute: "2-digit" }).format(new Date(time));
}

function timelineBucket(session: SessionView) {
  const time = sessionTime(session);
  if (!time) return { key: "older", label: t("chat.timelineOlder") };
  const date = new Date(time);
  const now = new Date();
  const startToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  const startYesterday = startToday - 24 * 60 * 60 * 1000;
  const value = date.getTime();
  if (value >= startToday) return { key: "today", label: t("chat.timelineToday") };
  if (value >= startYesterday) return { key: "yesterday", label: t("chat.timelineYesterday") };
  const days = (Date.now() - value) / (24 * 60 * 60 * 1000);
  if (days <= 7) return { key: "seven", label: t("chat.timelineSevenDays") };
  if (days <= 30) return { key: "thirty", label: t("chat.timelineThirtyDays") };
  return { key: "older", label: t("chat.timelineOlder") };
}

function sessionTime(session: SessionView) {
  const raw = session.timeline_at || session.at;
  const value = raw ? new Date(raw).getTime() : 0;
  return Number.isFinite(value) ? value : 0;
}

function toggleID(ids: Set<string>, id: string) {
  const next = new Set(ids);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  return next;
}

function loadIDs(key: string) {
  try {
    const value = JSON.parse(localStorage.getItem(key) || "[]");
    return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
  } catch {
    return [];
  }
}

function saveIDs(key: string, ids: Set<string>) {
  localStorage.setItem(key, JSON.stringify([...ids]));
}
</script>

<style scoped>
.chat-side--right {
  width: 300px;
  min-width: 280px;
  flex: 0 0 300px;
}

.chat-session-item {
  min-height: 58px;
  color: var(--q-dark);
}

:global(.body--dark) .chat-session-item {
  color: rgba(248, 250, 252, 0.92);
}

.chat-session-title {
  margin-top: 4px;
  color: inherit;
  font-weight: 700;
  line-height: 1.35;
}

.chat-timeline-label {
  padding: 12px 8px 6px;
  color: rgba(102, 112, 133, 0.78);
  font-size: 12px;
  font-weight: 800;
}

:global(.body--dark) .chat-timeline-label {
  color: rgba(203, 213, 225, 0.78);
}

.chat-session-time-badge {
  background: rgba(25, 118, 210, 0.1);
  color: #2563eb;
  font-size: 10px;
  font-weight: 800;
}

:global(.body--dark) .chat-session-time-badge {
  background: rgba(147, 197, 253, 0.18);
  color: #dbeafe;
}

.chat-session-menu-section {
  padding-left: 4px;
}

.chat-session-menu-btn {
  color: rgba(102, 112, 133, 0.92);
}

:global(.body--dark) .chat-session-menu-btn {
  color: rgba(248, 250, 252, 0.82);
}

:global(.body--dark) .chat-session-time {
  color: rgba(203, 213, 225, 0.76) !important;
}

.chat-session-item--active,
:global(.body--dark) .chat-session-item--active {
  color: #fff !important;
}

.chat-session-item--active .chat-session-time,
.chat-session-item--active .chat-action-btn,
.chat-session-item--active .chat-session-menu-btn {
  color: rgba(255, 255, 255, 0.86) !important;
}

.chat-session-item--active .chat-session-time-badge {
  background: rgba(255, 255, 255, 0.2);
  color: #fff;
}

:global(.body--dark) .chat-action-btn {
  background: rgba(15, 23, 42, 0.34);
}
</style>
