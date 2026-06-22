<template>
  <q-btn round flat class="app-header-icon-btn" icon="notifications_none" size="md" aria-label="Notifications">
    <q-badge v-if="unreadCount > 0" color="negative" floating rounded>{{ unreadCount }}</q-badge>
    <q-menu anchor="bottom right" self="top right" :offset="[0, 8]" class="inbound-notification-menu">
      <q-list dense>
        <q-item>
          <q-item-section class="text-weight-medium">{{ t('chat.inboundNotify.title', '渠道通知') }}</q-item-section>
          <q-item-section side>
            <q-btn
              v-if="items.length"
              flat
              dense
              size="sm"
              :label="t('chat.inboundNotify.markAllRead', '全部已读')"
              @click="emit('markAllRead')"
            />
          </q-item-section>
        </q-item>
        <q-separator />
        <q-item v-if="!items.length">
          <q-item-section class="text-caption text-grey">{{
            t('chat.inboundNotify.empty', '暂无通知')
          }}</q-item-section>
        </q-item>
        <q-item
          v-for="item in items"
          :key="item.id"
          v-close-popup
          clickable
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
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import type { InboundNotification } from '../../stores/inboundNotifications';

defineProps<{
  items: InboundNotification[];
  unreadCount: number;
}>();

const emit = defineEmits<{
  openSession: [sessionId: string, agentId: string];
  markRead: [id: string];
  markAllRead: [];
}>();

const { t } = useI18n();
const $q = useQuasar();

function formatTs(ts: number): string {
  const d = new Date(ts);
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function onOpen(item: InboundNotification) {
  emit('markRead', item.id);
  emit('openSession', item.sessionId, item.agentId);
}
</script>

<style>
.inbound-notification-menu {
  min-width: min(320px, 90vw);
  max-width: min(400px, 90vw);
}
</style>
