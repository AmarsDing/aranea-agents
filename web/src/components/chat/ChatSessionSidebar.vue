<template>
  <transition name="chat-side">
    <aside v-show="open" class="chat-side chat-side--right column no-wrap">
      <div class="chat-session-header row items-center justify-between no-wrap">
        <div class="text-caption text-cream-muted text-uppercase chat-session-header__label">
          Session
        </div>
        <span class="chat-session-count-badge">{{ sessions.length }}</span>
      </div>
      <q-scroll-area class="col chat-session-scroll">
        <q-list class="chat-session-list" dense>
          <template v-for="group in timelineGroups" :key="group.key">
            <q-item-label header class="chat-timeline-label">
              {{ group.label }}
            </q-item-label>
            <q-item
              v-for="session in group.sessions"
              :key="session.id"
              clickable
              class="chat-session-item rounded-borders"
              :class="{ 'chat-session-item--active': selectedSessionId === session.id }"
              @click="$emit('select', session.id)"
            >
              <q-item-section class="chat-session-main">
                <div class="chat-session-title-row row items-center no-wrap">
                  <div class="chat-session-title-wrap col">
                    <div class="chat-session-title">
                      {{ session.title }}
                    </div>
                    <q-tooltip
                      v-if="session.title"
                      anchor="top middle"
                      self="bottom middle"
                      :offset="[0, 8]"
                      :delay="300"
                      content-class="chat-session-title-tooltip"
                    >
                      {{ session.title }}
                    </q-tooltip>
                  </div>
                  <q-icon
                    v-if="isFavorite(session.id)"
                    class="chat-session-fav col-auto"
                    name="star"
                    color="amber-7"
                    size="16px"
                  />
                </div>
                <div class="chat-session-meta-row row items-center no-wrap">
                  <q-badge
                    class="chat-session-time-badge"
                    rounded
                    :label="shortTime(session)"
                  />
                  <q-circular-progress
                    :value="session.context_used_ratio * 100"
                    show-value
                    size="32px"
                    :thickness="0.22"
                    :color="sessionProgressColor(session.id)"
                    track-color="transparent"
                    class="chat-session-progress"
                  >
                    <span class="chat-session-progress__label">
                      {{ Math.round(session.context_used_ratio * 100) }}%
                    </span>
                  </q-circular-progress>
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
                </div>
              </q-item-section>
            </q-item>
          </template>
        </q-list>
      </q-scroll-area>
      <q-separator class="cream-sep" />
      <div class="chat-session-actions">
        <q-btn
          unelevated
          dense
          class="chat-primary-btn"
          color="primary"
          no-caps
          :label="t('chat.newSession')"
          @click="$emit('new-session')"
        />
        <q-btn
          outline
          dense
          class="chat-outline-danger-btn"
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

function sessionProgressColor(sessionId: string) {
  const active = props.selectedSessionId === sessionId;
  if (active) return props.isDark ? "cyan-4" : "amber-8";
  return props.isDark ? "blue-grey-5" : "brown-5";
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
  width: var(--chat-side-right-width, 360px);
  min-width: min(var(--chat-side-right-width, 360px), 100%);
  max-width: 100%;
  flex: 0 0 var(--chat-side-right-width, 360px);
  box-sizing: border-box;
  overflow-x: hidden;
}

.chat-session-scroll {
  width: 100%;
  max-width: 100%;
  min-width: 0;
  flex: 1 1 auto;
}

.chat-session-scroll :deep(.q-scrollarea__container) {
  overflow-x: hidden !important;
}

.chat-session-scroll :deep(.q-scrollarea__content) {
  width: 100% !important;
  max-width: 100% !important;
  overflow-x: hidden !important;
}

.chat-session-scroll :deep(.q-scrollarea__bar--h),
.chat-session-scroll :deep(.q-scrollarea__thumb--h) {
  display: none !important;
  height: 0 !important;
  opacity: 0% !important;
}

.chat-session-list {
  width: 100%;
  max-width: 100%;
  box-sizing: border-box;
  padding: 6px 10px 10px !important;
}

.chat-session-list :deep(.q-item) {
  width: 100%;
  max-width: 100%;
  min-width: 0;
  overflow: hidden;
  box-sizing: border-box;
}

.chat-session-list :deep(.q-item__section--main) {
  min-width: 0 !important;
  max-width: 100%;
}

.chat-session-list :deep(.q-item__label--header) {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
}

.chat-session-header__label {
  letter-spacing: 0.08em;
  font-weight: 700;
  font-size: 13px;
}

.chat-session-count-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  min-width: 40px;
  height: 32px;
  padding: 0 14px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--color-accent) 16%, var(--glass-elevated));
  color: var(--color-text-primary);
  border: 1px solid color-mix(in srgb, var(--color-accent) 28%, var(--glass-border));
  font-size: 15px;
  font-weight: 800;
  line-height: 1;
  letter-spacing: 0.02em;
  box-shadow: var(--glass-inner-highlight);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
}

:global(.body--dark) .chat-session-count-badge {
  color: var(--color-accent);
  border-color: color-mix(in srgb, var(--color-accent) 48%, var(--glass-border));
  background: color-mix(in srgb, var(--color-accent) 22%, var(--glass-elevated));
  box-shadow: var(--glass-inner-highlight), 0 0 14px rgb(0 229 255 / 12%);
}

.chat-session-item {
  width: 100%;
  max-width: 100%;
  min-height: 64px;
  padding: 10px 12px !important;
  margin-bottom: 8px;
  color: var(--color-text-secondary);
  overflow: hidden;
  box-sizing: border-box;
  border-radius: 14px !important;
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
}

.chat-session-main {
  min-width: 0;
  max-width: 100%;
  padding: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.chat-session-title-row {
  width: 100%;
  min-width: 0;
  gap: 6px;
}

.chat-session-title-wrap {
  min-width: 0;
  flex: 1 1 auto;
  overflow: hidden;
}

.chat-session-meta-row {
  width: 100%;
  gap: 8px;
  justify-content: flex-start;
}

.chat-session-fav {
  flex-shrink: 0;
}

.chat-session-title {
  display: block;
  width: 100%;
  max-width: 100%;
  color: var(--color-text-secondary);
  font-size: 15px;
  font-weight: 700;
  line-height: 1.4;
  text-align: left;
  direction: ltr;
  unicode-bidi: plaintext;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.chat-session-item--active .chat-session-title {
  color: var(--color-text-heading);
}

:global(.body--dark) .chat-session-item--active .chat-session-title {
  color: var(--color-text-primary);
}

.chat-session-item:not(.chat-session-item--active) .chat-session-title {
  color: var(--color-text-secondary);
}

.chat-session-item:hover:not(.chat-session-item--active) .chat-session-title {
  color: var(--color-text-primary);
}

:global(.chat-session-title-tooltip) {
  max-width: min(400px, 92vw);
  padding: 12px 14px !important;
  font-size: 18px !important;
  font-weight: 600;
  line-height: 1.45;
  overflow-wrap: anywhere;
  white-space: normal;
  background: var(--glass-elevated) !important;
  color: var(--color-text-heading) !important;
  border: 1px solid var(--glass-border) !important;
  border-radius: 14px;
  box-shadow: var(--glass-inner-highlight);
  backdrop-filter: blur(var(--glass-blur-elevated));
  -webkit-backdrop-filter: blur(var(--glass-blur-elevated));
}

:global(.chat-session-title-tooltip .q-tooltip__content) {
  font-size: 18px;
  line-height: 1.45;
}

:global(.body--dark .chat-session-title-tooltip) {
  background: color-mix(in srgb, var(--glass-elevated) 92%, var(--canvas-base)) !important;
  color: var(--color-text-heading) !important;
  border: 1px solid var(--glass-border-hover) !important;
}

.chat-session-progress__label {
  font-size: 12px;
  font-weight: 800;
  line-height: 1;
  color: var(--color-text-tertiary);
}

.chat-session-item--active .chat-session-progress__label {
  color: var(--color-accent);
}

.chat-session-item:not(.chat-session-item--active) .chat-session-progress__label {
  color: var(--color-text-tertiary);
}

.chat-timeline-label {
  padding: 14px 10px 8px;
  color: var(--color-text-secondary);
  font-size: 13px;
  font-weight: 800;
  letter-spacing: 0.04em;
}

:global(.body--dark) .chat-timeline-label {
  color: var(--color-text-secondary);
}

.chat-session-time-badge {
  max-width: 64px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  background: color-mix(in srgb, var(--glass-surface-hover) 88%, transparent);
  color: var(--color-text-tertiary);
  border: 1px solid var(--glass-border);
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.02em;
  padding: 4px 8px;
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}

.chat-session-item--active .chat-session-time-badge {
  background: color-mix(in srgb, var(--color-accent) 12%, var(--glass-surface-hover));
  border-color: color-mix(in srgb, var(--color-accent) 35%, var(--glass-border));
  color: var(--color-accent);
}

.chat-session-item:not(.chat-session-item--active) .chat-session-time-badge {
  color: var(--color-text-tertiary);
  background: color-mix(in srgb, var(--glass-surface) 75%, transparent);
}

:global(.body--dark) .chat-session-item:not(.chat-session-item--active) .chat-session-time-badge {
  color: var(--color-text-slate-400);
  background: color-mix(in srgb, var(--glass-surface) 88%, transparent);
  border-color: var(--glass-border);
}

.chat-session-menu-btn {
  flex-shrink: 0;
  width: 32px;
  height: 32px;
  min-height: 32px;
  margin-left: auto;
  color: var(--color-icon-muted);
  background: color-mix(in srgb, var(--glass-surface-hover) 85%, transparent);
  border: 1px solid var(--glass-border);
}

.chat-session-item--active .chat-session-menu-btn {
  color: var(--color-accent);
  border-color: color-mix(in srgb, var(--color-accent) 35%, var(--glass-border));
  background: color-mix(in srgb, var(--color-accent) 10%, var(--glass-surface-hover));
}

.chat-session-item:not(.chat-session-item--active) .chat-session-menu-btn {
  color: var(--color-icon-muted);
}

.chat-session-item:hover:not(.chat-session-item--active) .chat-session-menu-btn {
  color: var(--color-text-secondary);
  border-color: var(--glass-border-hover);
  background: var(--glass-surface-hover);
}

:global(.body--dark) .chat-session-item:not(.chat-session-item--active) .chat-session-menu-btn {
  color: var(--color-text-slate-400);
  background: color-mix(in srgb, var(--glass-surface) 90%, transparent);
}

.chat-session-item--active {
  box-shadow: var(--glass-inner-highlight), inset 3px 0 0 var(--color-accent) !important;
}

:global(.body--dark) .chat-session-item--active {
  box-shadow: var(--glass-inner-highlight), inset 3px 0 0 color-mix(in srgb, var(--color-accent) 72%, transparent) !important;
}

@media (width <= 900px) {
  .chat-side--right {
    width: var(--chat-side-right-width, 320px);
    min-width: min(var(--chat-side-right-width, 320px), 100%);
    flex: 0 0 var(--chat-side-right-width, 320px);
  }
}
</style>
