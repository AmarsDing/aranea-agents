<template>
  <!-- 最新邮件：只列最近几封，点了去邮件页。接口没有「未读 / 需行动」的标记，这里不编造。 -->
  <section class="today-mail">
    <header class="today-panel__head">
      <h2 class="today-panel__title">
        {{ t('today.mail.title') }}
        <span v-if="status === 'ok'" class="today-panel__count">{{ total }}</span>
      </h2>
      <router-link class="today-link" to="/mail">{{ t('today.mail.viewAll') }}</router-link>
    </header>
    <p v-if="status === 'denied'" class="today-empty">{{ t('today.mail.denied') }}</p>
    <p v-else-if="status === 'error'" class="today-note today-note--danger" role="alert">
      {{ t('today.mail.failed') }}
    </p>
    <p v-else-if="status === 'ok' && mails.length === 0" class="today-empty">{{ t('today.mail.empty') }}</p>
    <ul v-else class="today-tasks__list">
      <li v-for="mail in mails" :key="mail.id">
        <router-link class="today-mail__row" to="/mail">
          <span class="today-mail__from">{{ senderName(mail.sender) }}</span>
          <span class="today-mail__time today-mono">{{ timeLabel(mail.received_at) }}</span>
          <span class="today-mail__subject">{{ mail.subject || t('today.mail.noSubject') }}</span>
          <span v-if="mail.task_ref" class="today-tag">{{ t('today.mail.hasTask') }}</span>
        </router-link>
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { MailMessageRow } from '../../features/office/types';
import { clock, dayBounds } from '../../features/today/logic';
import type { BlockStatus } from '../../features/today/useTodaySide';

defineProps<{ mails: MailMessageRow[]; total: number; status: BlockStatus }>();

const { t } = useI18n();

/** 「张三 <a@b.c>」只显示名字；没有名字时显示地址。 */
function senderName(sender: string): string {
  return sender.replace(/\s*<[^>]*>\s*$/, '').replace(/^"|"$/g, '') || sender;
}

/** 今天的显示时刻，更早的显示月-日。 */
function timeLabel(iso: string): string {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return '';
  if (at.getTime() >= dayBounds(new Date()).start.getTime()) return clock(iso);
  return `${String(at.getMonth() + 1).padStart(2, '0')}-${String(at.getDate()).padStart(2, '0')}`;
}
</script>
