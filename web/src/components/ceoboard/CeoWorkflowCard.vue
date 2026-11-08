<script setup lang="ts">
// 工作流节点全表（整合 P2-8，自 ceo-dashboard WorkflowCard.vue 移植）——
// 值班视图的工程细节呈现：节点行 + 状态说明 + 指标 + 数据源双轨。
import { useI18n } from 'vue-i18n';
import type { NodeView, WorkflowView } from '../../features/ceoboard/types';

defineProps<{ wf: WorkflowView }>();
const emit = defineEmits<{ openNode: [nodeId: string] }>();
const { t } = useI18n();

function fmtTime(at?: string): string {
  if (!at) return '';
  const d = new Date(at);
  if (d.getFullYear() < 2000) return t('ceoboard.common.neverOk');
  return d.toLocaleTimeString();
}
function nodeTitle(n: NodeView): string {
  const parts = [n.reason];
  if (n.source?.skip_reason) parts.push(t('ceoboard.wf.tipSkip', { msg: n.source.skip_reason }));
  if (n.source?.last_err) parts.push(t('ceoboard.wf.tipErr', { msg: n.source.last_err }));
  if (n.source?.last_ok_at) parts.push(t('ceoboard.wf.tipLastOk', { at: fmtTime(n.source.last_ok_at) }));
  return parts.filter(Boolean).join('\n');
}
function originLabel(origin: string): string {
  return origin === 'event'
    ? t('ceoboard.wf.originEvent')
    : origin === 'reconcile'
      ? t('ceoboard.wf.originReconcile')
      : t('ceoboard.wf.originBoth');
}
</script>

<template>
  <div class="ceo-wf" :class="wf.health">
    <div class="ceo-wf-head">
      <span class="ceo-dot" :class="wf.health"></span>
      <span class="name">{{ wf.name }}</span>
      <span class="ceo-mono ceo-dim">{{ wf.id }} v{{ wf.version }}</span>
      <span class="meta">{{ t('ceoboard.wf.ownerPerms', { owner: wf.owner, perms: wf.permissions.join('/') }) }}</span>
      <span class="meta ceo-wf-head-right">
        {{ t('ceoboard.wf.headMeta', { alerts: wf.alerts, volume: wf.volume_1h }) }}
      </span>
    </div>
    <table class="ceo-nodes">
      <thead>
        <tr>
          <th class="ceo-th-dot"></th>
          <th>{{ t('ceoboard.wf.thNode') }}</th>
          <th>{{ t('ceoboard.wf.thReason') }}</th>
          <th>{{ t('ceoboard.wf.thMetrics') }}</th>
          <th class="ceo-th-source">{{ t('ceoboard.wf.thSource') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="n in wf.nodes"
          :key="n.id"
          class="ceo-node-row"
          tabindex="0"
          :title="nodeTitle(n)"
          @click="emit('openNode', n.id)"
          @keydown.enter.prevent="emit('openNode', n.id)"
          @keydown.space.prevent="emit('openNode', n.id)"
        >
          <td><span class="ceo-dot" :class="n.health"></span></td>
          <td>
            {{ n.order }}. {{ n.name }}<span class="ceo-dim ceo-mono"> {{ n.id }}</span>
          </td>
          <td>
            <span class="ceo-reason" :class="n.health">{{ n.reason }}</span>
            <span v-if="n.source?.skipped" class="ceo-badge-skip">{{ t('ceoboard.wf.badgeSkip') }}</span>
            <span
              v-if="n.source?.origin"
              class="ceo-badge-origin"
              :title="t('ceoboard.wf.originTitle', { origin: n.source.origin })"
            >
              {{ originLabel(n.source.origin) }}
            </span>
            <div v-if="n.source?.last_ok_at" class="ceo-dim ceo-subline">
              {{ t('ceoboard.wf.lastOkLine', { at: fmtTime(n.source.last_ok_at) })
              }}<span v-if="n.source.as_of"> · {{ t('ceoboard.wf.asOfPart', { at: fmtTime(n.source.as_of) }) }}</span>
            </div>
            <div v-if="n.source?.last_event_at" class="ceo-dim ceo-subline">
              {{ t('ceoboard.wf.eventLine', { at: fmtTime(n.source.last_event_at), count: n.source.msg_count })
              }}<span v-if="n.source.dup_count"> {{ t('ceoboard.wf.dupPart', { count: n.source.dup_count }) }}</span>
            </div>
          </td>
          <td>
            <span v-for="m in n.metrics" :key="m.id" class="ceo-metric" :title="m.note ?? m.id">
              <span class="ceo-dim">{{ m.id.replace(/^m\./, '') }}</span>
              <span class="v" :class="{ pending: m.pending }">
                {{ m.display }}<template v-if="m.pending">{{ t('ceoboard.wf.pending') }}</template>
              </span>
            </span>
          </td>
          <td class="ceo-dim ceo-mono ceo-src-cell">
            {{ n.source?.adapter ?? '—' }}
            <template v-if="n.source?.rest_path"><br />{{ n.source.rest_path }}</template>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
