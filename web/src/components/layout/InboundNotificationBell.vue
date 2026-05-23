<template>
  <q-btn round flat class="app-header-icon-btn" icon="notifications_none" size="md" aria-label="Notifications">
    <q-badge v-if="store.unreadCount > 0" color="negative" floating rounded>{{ store.unreadCount }}</q-badge>
    <q-menu anchor="bottom right" self="top right" :offset="[0, 8]" style="min-width: 320px; max-width: 400px">
      <q-list dense>
        <q-item>
          <q-item-section class="text-weight-medium">{{ t("chat.inboundNotify.title", "渠道通知") }}</q-item-section>
          <q-item-section side>
            <q-btn
              v-if="store.items.length"
              flat
              dense
              size="sm"
              :label="t('chat.inboundNotify.markAllRead', '全部已读')"
              @click="store.markAllRead()"
            />
          </q-item-section>
        </q-item>
        <q-separator />
        <q-item v-if="!store.items.length">
          <q-item-section class="text-caption text-grey">{{ t("chat.inboundNotify.empty", "暂无通知") }}</q-item-section>
        </q-item>
        <q-item
          v-for="item in store.items"
          :key="item.id"
          clickable
          v-close-popup
          :class="{ 'bg-grey-2': !item.read && !$q.dark.isActive, 'bg-grey-9': !item.read && $q.dark.isActive }"
          @click="onOpen(item)"
        >
          <q-item-section>
            <q-item-label>{{ item.title }}</q-item-label>
            <q-item-label caption lines="2">{{ item.preview }}</q-item-label>
          </q-item-section>
          <q-item-section side top>
            <q-item-label caption>{{ formatTs(item.ts) }}</q-item-label>
          </q-item-section>
        </q-item>
      </q-list>
    </q-menu>
  </q-btn>
</template>

<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import { useInboundNotificationStore, type InboundNotification } from "../../stores/inboundNotifications";

const props = defineProps<{
  onOpenSession: (sessionId: string, agentId: string) => void;
}>();

const { t } = useI18n();
const $q = useQuasar();
const store = useInboundNotificationStore();

function formatTs(ts: number): string {
  const d = new Date(ts);
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function onOpen(item: InboundNotification) {
  store.markRead(item.id);
  props.onOpenSession(item.sessionId, item.agentId);
}
</script>
