<script setup lang="ts">
// 节点明细抽屉（整合 P2-8，自 ceo-dashboard NodeDrawer.vue 移植）。
// 无障碍加固（AGENTS.md §6.11）：role=dialog + aria-modal、Esc 关闭、
// 打开时聚焦关闭按钮、关闭后焦点归还触发元素。
import { onMounted, onUnmounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { NodeDetail } from '../../features/ceoboard/types';

defineProps<{ detail: NodeDetail }>();
const emit = defineEmits<{ close: [] }>();
const { t } = useI18n();

const closeBtn = ref<HTMLButtonElement | null>(null);
let previouslyFocused: Element | null = null;

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.stopPropagation();
    emit('close');
  }
}

onMounted(() => {
  previouslyFocused = document.activeElement;
  closeBtn.value?.focus();
  window.addEventListener('keydown', onKeydown);
});
onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown);
  if (previouslyFocused instanceof HTMLElement) previouslyFocused.focus();
});

function fmt(at?: string): string {
  if (!at) return '—';
  return new Date(at).toLocaleString();
}
function originLabel(origin: string): string {
  return origin === 'event'
    ? t('ceoboard.drawer.originEvent')
    : origin === 'reconcile'
      ? t('ceoboard.drawer.originReconcile')
      : t('ceoboard.drawer.originBoth');
}
</script>

<template>
  <div class="ceo-drawer-mask" @click="emit('close')"></div>
  <aside class="ceo-drawer" role="dialog" aria-modal="true" :aria-label="t('ceoboard.drawer.ariaLabel')">
    <button ref="closeBtn" class="ceo-close-btn" @click="emit('close')">{{ t('ceoboard.drawer.close') }}</button>
    <h2>
      <span class="ceo-dot" :class="detail.health"></span> {{ detail.workflow_name }} / {{ detail.name }}
    </h2>
    <div class="ceo-dim ceo-mono ceo-drawer-id">{{ detail.workflow_id }} / {{ detail.id }}</div>
    <div v-if="detail.fetch_err" class="ceo-alert yellow ceo-drawer-degraded" role="alert">
      <span class="lvl">{{ t('ceoboard.drawer.degradedBanner') }}</span><span>{{ detail.fetch_err }}</span>
    </div>

    <section>
      <h3>{{ t('ceoboard.drawer.secTime') }}</h3>
      <div class="ceo-kv">
        <span class="k">as_of</span><span class="ceo-mono">{{ fmt(detail.source?.as_of ?? detail.as_of) }}</span>
        <span class="k">{{ t('ceoboard.drawer.kLastOk') }}</span><span class="ceo-mono">{{ fmt(detail.source?.last_ok_at) }}</span>
        <span class="k">{{ t('ceoboard.drawer.kStatus') }}</span><span>{{ detail.reason }}</span>
      </div>
    </section>

    <section>
      <h3>{{ t('ceoboard.drawer.secEnterExit') }}</h3>
      <div class="ceo-kv">
        <span class="k">enter</span><span class="ceo-mono">{{ detail.enter }}</span>
        <span class="k">exit</span><span class="ceo-mono">{{ detail.exit }}</span>
      </div>
    </section>

    <section>
      <h3>{{ t('ceoboard.drawer.secSource') }}</h3>
      <div v-if="detail.source" class="ceo-kv">
        <span class="k">{{ t('ceoboard.drawer.kAdapter') }}</span><span class="ceo-mono">{{ detail.source.adapter }}</span>
        <template v-if="detail.source.origin">
          <span class="k">{{ t('ceoboard.drawer.kOrigin') }}</span>
          <span>{{ originLabel(detail.source.origin) }}{{ t('ceoboard.drawer.originNote') }}</span>
        </template>
        <template v-if="detail.source.stream">
          <span class="k">NATS stream</span>
          <span class="ceo-mono">
            {{ detail.source.stream
            }}{{ t('ceoboard.drawer.consumerSuffix', { state: detail.source.consuming ? t('ceoboard.drawer.consumerUp') : t('ceoboard.drawer.consumerDown') }) }}
          </span>
        </template>
        <template v-if="detail.source.stream_missing">
          <span class="k">{{ t('ceoboard.drawer.kDrift') }}</span>
          <span v-if="detail.source.stream_optional" class="ceo-dim">{{ t('ceoboard.drawer.driftOptional') }}</span>
          <span v-else class="ceo-text-red">{{ t('ceoboard.drawer.driftMissing') }}</span>
        </template>
        <template v-if="detail.source.backlog">
          <span class="k">{{ t('ceoboard.drawer.kBacklog') }}</span>
          <span class="ceo-text-yellow">{{ t('ceoboard.drawer.backlogNote') }}</span>
        </template>
        <template v-if="detail.source.rest_path">
          <span class="k">{{ t('ceoboard.drawer.kRest') }}</span><span class="ceo-mono">{{ detail.source.rest_path }}</span>
        </template>
        <template v-if="detail.source.last_event_at">
          <span class="k">{{ t('ceoboard.drawer.kLastEvent') }}</span>
          <span class="ceo-mono">
            {{ fmt(detail.source.last_event_at)
            }}{{ t('ceoboard.drawer.lastEventNote', { count: detail.source.msg_count, dup: detail.source.dup_count ?? 0 }) }}
          </span>
        </template>
        <template v-if="detail.source.last_err">
          <span class="k">{{ t('ceoboard.drawer.kLastErr') }}</span>
          <span class="ceo-text-red">
            {{ detail.source.last_err }}{{ t('ceoboard.drawer.lastErrNote', { count: detail.source.fail_count }) }}
          </span>
        </template>
        <template v-if="detail.source.skip_reason">
          <span class="k">{{ t('ceoboard.drawer.kSkipReason') }}</span>
          <span class="ceo-text-yellow">{{ detail.source.skip_reason }}</span>
        </template>
      </div>
      <div v-else class="ceo-empty">{{ t('ceoboard.drawer.noSource') }}</div>
    </section>

    <section v-if="detail.poll_snippet">
      <h3>{{ t('ceoboard.drawer.secPoll') }}</h3>
      <pre class="ceo-snip ceo-mono">{{ detail.poll_snippet }}</pre>
    </section>

    <section v-if="detail.events">
      <h3>{{ t('ceoboard.drawer.secWebhook') }}</h3>
      <div v-if="!detail.events.length" class="ceo-empty">{{ t('ceoboard.drawer.webhookEmpty') }}</div>
      <div v-for="e in detail.events" :key="e.key" class="ceo-approval-item">
        <span class="ceo-tag ceo-mono">{{ e.event || t('ceoboard.drawer.webhookNoEvent') }}</span>
        <span v-if="e.status === 'failed'" class="ceo-tag ceo-text-red">{{ t('ceoboard.drawer.webhookFailed') }}</span>
        <span class="ceo-mono">{{ e.entity_id || e.key }}</span>
        <span class="ceo-dim">{{ fmt(e.at) }}</span>
      </div>
    </section>

    <section v-if="detail.nats_events">
      <h3>{{ t('ceoboard.drawer.secNats') }}</h3>
      <div v-if="!detail.nats_events.length" class="ceo-empty">{{ t('ceoboard.drawer.natsEmpty') }}</div>
      <div v-for="e in detail.nats_events" :key="e.event_uid" class="ceo-approval-item">
        <span class="ceo-tag ceo-mono">{{ e.subject }}</span>
        <span class="ceo-mono">{{ e.event_id }}</span>
        <span class="ceo-dim">{{ fmt(e.received_at) }}</span>
      </div>
    </section>

    <section>
      <h3>{{ t('ceoboard.drawer.secDrill') }}</h3>
      <div v-if="!detail.drill?.length" class="ceo-empty">{{ t('ceoboard.drawer.drillEmpty') }}</div>
      <div v-for="d in detail.drill" :key="d.label" class="ceo-approval-item ceo-drill-item">
        <div><span class="ceo-tag">{{ d.type }}</span> {{ d.label }}</div>
        <div class="ceo-dim ceo-mono ceo-drill-path">{{ d.path }}</div>
        <div v-if="d.note" class="ceo-dim ceo-drill-note">{{ d.note }}</div>
        <div v-if="d.empty_state" class="ceo-empty ceo-drill-note">
          {{ t('ceoboard.drawer.drillEmptyState', { text: d.empty_state }) }}
        </div>
      </div>
    </section>

    <section>
      <h3>{{ t('ceoboard.drawer.secMetrics') }}</h3>
      <div v-for="m in detail.metrics" :key="m.id" class="ceo-kv">
        <span class="k ceo-mono">{{ m.id }}</span>
        <span>{{ m.display }}<span v-if="m.note" class="ceo-dim"> — {{ m.note }}</span></span>
      </div>
    </section>
  </aside>
</template>
