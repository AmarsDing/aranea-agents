<template>
  <q-btn round flat class="app-header-icon-btn" icon="inbox" size="md" :aria-label="t('notifyBell.ariaLabel')">
    <q-badge v-if="unreadCount > 0" color="negative" floating rounded>
      {{ unreadCount > 99 ? '99+' : unreadCount }}
    </q-badge>
    <q-menu anchor="bottom right" self="top right" :offset="[0, 8]" class="office-notification-menu">
      <q-list dense>
        <q-item>
          <q-item-section class="text-weight-medium">{{ t('notifyBell.title') }}</q-item-section>
          <q-item-section side>
            <q-btn
              v-if="items.length"
              flat
              dense
              size="sm"
              no-caps
              :label="t('notifyBell.markAllRead')"
              @click="emit('markAllRead')"
            />
          </q-item-section>
        </q-item>
        <q-separator />
        <q-item v-if="!items.length">
          <q-item-section class="text-caption text-grey">{{ t('notifyBell.empty') }}</q-item-section>
        </q-item>
        <q-item
          v-for="item in items"
          :key="item.id"
          v-close-popup
          clickable
          :class="{ 'office-notification-menu__unread': !item.read }"
          @click="onOpen(item)"
        >
          <q-item-section>
            <q-item-label>{{ item.title }}</q-item-label>
            <q-item-label v-if="item.body" caption lines="2">{{ item.body }}</q-item-label>
          </q-item-section>
          <q-item-section side top>
            <q-item-label caption class="mono">{{ formatTs(item.created_at) }}</q-item-label>
          </q-item-section>
        </q-item>
      </q-list>
    </q-menu>
  </q-btn>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import type { OfficeNotificationRow } from '../../features/office/types';

defineProps<{
  items: OfficeNotificationRow[];
  unreadCount: number;
}>();

const emit = defineEmits<{
  markRead: [id: string];
  markAllRead: [];
}>();

const { t } = useI18n();
const router = useRouter();

function formatTs(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
}

function onOpen(item: OfficeNotificationRow) {
  emit('markRead', item.id);
  if (item.link) {
    void router.push(item.link);
  }
}
</script>

<style scoped>
.office-notification-menu {
  min-width: min(340px, 90vw);
  max-width: min(420px, 90vw);
}

.office-notification-menu__unread {
  background: var(--nt-bg-2);
}

/* 时间戳等宽 + tabular-nums（AGENTS.md §6-9）。 */
.mono {
  font-family: var(--nt-font-mono);
  font-variant-numeric: tabular-nums;
}
</style>
