<template>
  <footer class="fleet-status" aria-label="运行台状态">
    <div v-for="u in state.usage" :key="u.agent" class="fleet-usage" :title="usageTitle(u)">
      <span class="fleet-usage__name">{{ agentLabel(u.agent) }}</span>
      <template v-for="w in u.windows" :key="w.label">
        <span
          v-if="w.percent >= 0"
          class="fleet-usage__bar"
          role="meter"
          :aria-valuenow="Math.round(w.percent)"
          aria-valuemin="0"
          aria-valuemax="100"
          :aria-label="`${w.label} 额度`"
        >
          <span
            class="fleet-usage__fill"
            :class="{ 'is-high': w.percent >= 80 }"
            :style="{ width: `${Math.min(100, w.percent)}%` }"
          />
        </span>
        <span class="mono"
          >{{ w.label }} {{ w.percent >= 0 ? `${Math.round(w.percent)}%` : tokens(w.tokens)
          }}<template v-if="w.resetsAt"> · {{ resetIn(w.resetsAt) }}</template></span
        >
      </template>
    </div>
    <span v-if="state.usage.length === 0" class="fleet-status__muted"
      >本机还没有 Claude Code / Codex 会话日志，暂无用量</span
    >
    <q-space />
    <span v-if="working" class="fleet-status__count"><q-spinner size="12px" /> {{ working }} 个在干活</span>
    <button v-if="needs" type="button" class="fleet-status__count is-warn" @click="state.view = 'board'">
      <q-icon name="help" size="14px" /> {{ needs }} 个等你
    </button>
    <button type="button" class="fleet-status__count fleet-status__tools" @click="agentsOpen = true">
      <q-icon name="download" size="14px" /> CLI 工具
    </button>
    <FleetAgentsDialog v-model="agentsOpen" />
    <button
      type="button"
      class="fleet-status__bell"
      :aria-label="`动态，${state.unread} 条未读`"
      @click="state.view = 'feed'"
    >
      <q-icon name="notifications" size="16px" />
      <span v-if="state.unread" class="fleet-status__dot mono">{{ state.unread > 99 ? '99+' : state.unread }}</span>
    </button>
  </footer>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useRoute } from 'vue-router';
import type { AgentUsage } from '../api';
import { useFleet } from '../useFleet';
import FleetAgentsDialog from './FleetAgentsDialog.vue';

const { state, agentLabel } = useFleet();
// 编码工具页「去安装」带 ?panel=cli 过来时直接打开 CLI 工具面板
const agentsOpen = ref(useRoute().query.panel === 'cli');

const working = computed(() => state.terminals.filter((t) => t.status === 'working').length);
const needs = computed(() => state.terminals.filter((t) => t.status === 'needs_input').length);

function tokens(n: number): string {
  if (n >= 1e9) return `${(n / 1e9).toFixed(1)}B`;
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)}M`;
  if (n >= 1e3) return `${(n / 1e3).toFixed(0)}K`;
  return String(n);
}

function resetIn(iso: string): string {
  const ms = Date.parse(iso) - Date.now();
  if (Number.isNaN(ms) || ms <= 0) return '';
  const h = Math.floor(ms / 3600000);
  const m = Math.floor((ms % 3600000) / 60000);
  return h >= 24 ? `${Math.floor(h / 24)}天${h % 24}时后重置` : `${h}时${m}分后重置`;
}

function usageTitle(u: AgentUsage): string {
  const ws = u.windows
    .map((w) => `${w.label}：${tokens(w.tokens)} tokens${w.percent >= 0 ? `，额度 ${w.percent.toFixed(0)}%` : ''}`)
    .join('；');
  return `${agentLabel(u.agent)}（${u.sessions} 个会话，来源 ${u.source}）${ws}`;
}
</script>

<style scoped lang="sass">
.fleet-status
  display: flex
  align-items: center
  gap: var(--nt-space-4)
  min-height: 30px
  padding: 0 var(--nt-space-3)
  border-top: 1px solid var(--nt-border-1)
  background: var(--nt-bg-1)
  color: var(--nt-text-2)
  font-size: var(--nt-fs-micro)
.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums
.fleet-usage
  display: flex
  align-items: center
  gap: var(--nt-space-2)
.fleet-usage__name
  color: var(--nt-text-1)
.fleet-usage__bar
  position: relative
  width: 48px
  height: 4px
  overflow: hidden
  border-radius: var(--nt-radius-pill)
  background: var(--nt-bg-3)
.fleet-usage__fill
  position: absolute
  inset: 0 auto 0 0
  background: var(--nt-accent)
  &.is-high
    background: var(--nt-warn)
.fleet-status__muted
  color: var(--nt-text-3)
.fleet-status__count
  display: inline-flex
  align-items: center
  gap: var(--nt-space-1)
  border: 0
  background: transparent
  color: var(--nt-accent)
  font-size: var(--nt-fs-micro)
  &.is-warn
    color: var(--nt-warn)
    cursor: pointer
.fleet-status__tools
  flex-shrink: 0
  white-space: nowrap
  color: var(--nt-text-1)
  cursor: pointer
  &:hover
    color: var(--nt-accent)
  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
.fleet-status__bell
  position: relative
  display: inline-flex
  align-items: center
  justify-content: center
  width: 30px
  height: 30px
  border: 0
  border-radius: var(--nt-radius-s)
  background: transparent
  color: var(--nt-text-1)
  cursor: pointer
  &:hover
    background: var(--nt-bg-2)
  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
.fleet-status__dot
  position: absolute
  top: 0
  right: -4px
  min-width: 16px
  padding: 0 3px
  border-radius: var(--nt-radius-pill)
  background: var(--nt-accent)
  color: var(--nt-on-accent)
  font-size: var(--nt-fs-micro)
  line-height: 16px
</style>
