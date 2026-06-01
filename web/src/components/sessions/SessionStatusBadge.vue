<template>
  <span class="session-status-badge" :class="`session-status-badge--${status}`" @mouseenter="hovered = true" @mouseleave="hovered = false">
    <q-spinner v-if="status === 'running'" size="14px" :color="iconColor" />
    <q-icon v-else :name="statusIcon" :color="iconColor" size="14px" />
    <span class="session-status-badge__label">{{ statusLabel }}</span>
    <q-tooltip v-if="tooltipContent" :delay="300" anchor="top middle" self="bottom middle" :offset="[0, 8]">
      {{ tooltipContent }}
    </q-tooltip>
  </span>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import type { SessionStatus, SessionStatusReason } from "../../features/session/types";

const props = defineProps<{
  status: SessionStatus;
  statusReason?: SessionStatusReason;
  statusChangedAt?: string;
}>();

const hovered = ref(false);

const statusConfig: Record<SessionStatus, { icon: string; label: string; color: string }> = {
  idle: { icon: "circle", label: "空闲", color: "grey-6" },
  running: { icon: "", label: "执行中", color: "accent" },
  completed: { icon: "check_circle", label: "已完成", color: "positive" },
  interrupted: { icon: "cancel", label: "已中断", color: "warning" },
  awaiting_confirmation: { icon: "pause_circle", label: "等待确认", color: "accent" },
};

const reasonLabels: Record<Exclude<SessionStatusReason, "">, string> = {
  user_cancelled: "用户取消",
  timeout: "执行超时",
  budget_escalated: "预算超限",
  error: "执行出错",
  context_overflow: "上下文溢出",
  server_shutdown: "服务关闭",
  unexpected_shutdown: "服务异常退出",
  confirmation_timeout: "确认超时",
  tool_confirmation: "工具执行确认",
  agent_awaiting_reply: "Agent 等待回复",
  manual_override: "手动覆盖",
};

const config = computed(() => statusConfig[props.status] ?? statusConfig.idle);
const statusIcon = computed(() => config.value.icon);
const statusLabel = computed(() => config.value.label);
const iconColor = computed(() => config.value.color);

const reasonLabel = computed(() => {
  if (!props.statusReason) return "";
  return reasonLabels[props.statusReason as Exclude<SessionStatusReason, "">] ?? props.statusReason;
});

const changedAtLabel = computed(() => {
  if (!props.statusChangedAt) return "";
  const d = new Date(props.statusChangedAt);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleString();
});

const tooltipContent = computed(() => {
  const parts: string[] = [];
  if (reasonLabel.value) parts.push(reasonLabel.value);
  if (changedAtLabel.value) parts.push(changedAtLabel.value);
  return parts.join(" · ");
});
</script>

<style scoped lang="sass">
.session-status-badge
  display: inline-flex
  align-items: center
  gap: var(--space-1)
  padding: 2px 8px
  border-radius: 999px
  font-size: var(--text-xs)
  font-weight: 700
  line-height: 1.4
  white-space: nowrap
  background: color-mix(in srgb, var(--glass-surface) 55%, transparent)
  border: 1px solid var(--glass-border)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))

.session-status-badge--idle
  color: var(--color-text-tertiary)

.session-status-badge--running
  color: var(--color-accent)
  border-color: color-mix(in srgb, var(--color-accent) 30%, var(--glass-border))
  background: color-mix(in srgb, var(--color-accent) 8%, var(--glass-surface))

.session-status-badge--completed
  color: var(--color-success)
  border-color: color-mix(in srgb, var(--color-success) 25%, var(--glass-border))
  background: color-mix(in srgb, var(--color-success) 8%, var(--glass-surface))

.session-status-badge--interrupted
  color: var(--color-warning)
  border-color: color-mix(in srgb, var(--color-warning) 25%, var(--glass-border))
  background: color-mix(in srgb, var(--color-warning) 8%, var(--glass-surface))

.session-status-badge--awaiting_confirmation
  color: var(--color-accent)
  border-color: color-mix(in srgb, var(--color-accent) 25%, var(--glass-border))
  background: color-mix(in srgb, var(--color-accent) 8%, var(--glass-surface))

.session-status-badge__label
  letter-spacing: 0.02em
</style>
