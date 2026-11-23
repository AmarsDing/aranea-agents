<template>
  <article class="tool-card" :class="{ 'is-off': !agent.enabled }">
    <header class="tool-card__head">
      <img v-if="icon" :src="icon" class="tool-card__logo" alt="" />
      <span v-else class="tool-card__logo tool-card__logo--letter" aria-hidden="true">{{ title.slice(0, 1) }}</span>
      <h3 class="tool-card__name">{{ title }}</h3>
      <span class="tool-card__status" :class="`is-${status}`">
        <i class="cap-dot" :class="DOT[status]" aria-hidden="true" />{{ t(`cap.coding.tool.status.${status}`) }}
      </span>
      <q-toggle
        dense
        color="primary"
        :model-value="!!agent.enabled"
        :disable="busy === 'toggle'"
        :aria-label="agent.enabled ? t('cap.common.batchDisable') : t('cap.common.batchEnable')"
        @update:model-value="emit('toggle', Boolean($event))"
      />
    </header>

    <!-- 一行事实：登录状态 · 权限 · 模型 · 更新方式 · 版本 · 更新时间 -->
    <p class="tool-card__facts">
      <span v-if="login" :class="`is-login-${login.kind}`" :title="login.text">
        <q-icon :name="LOGIN_ICON[login.kind]" size="14px" aria-hidden="true" />{{ login.text }}
      </span>
      <span :class="{ 'is-alert': permTone === 'alert', 'is-watch': permTone === 'watch' }" :title="permLabel">
        <q-icon :name="permIcon" size="14px" aria-hidden="true" />{{ permLabel }}
      </span>
      <span v-if="modelText" class="mono" :title="t('cap.coding.tool.edit.model')">
        <q-icon name="psychology" size="14px" aria-hidden="true" />{{ modelText }}
      </span>
      <span v-if="preset?.install">
        <q-icon :name="preset.autoUpdate ? 'autorenew' : 'update_disabled'" size="14px" aria-hidden="true" />{{
          preset.autoUpdate ? t('cap.coding.tool.autoUpdateOn') : t('cap.coding.tool.autoUpdateOff')
        }}
      </span>
      <span class="mono">{{
        preset?.version ? t('cap.coding.tool.version', { v: shortVersion }) : t('cap.coding.tool.noVersion')
      }}</span>
      <span v-if="updatedAt" class="mono">{{ t('cap.coding.tool.updatedAt', { at: updatedAt }) }}</span>
    </p>
    <!-- 更新中：进度条 + 安装日志最后一行 -->
    <div v-if="installing" class="tool-card__progress" role="status" aria-live="polite">
      <q-linear-progress
        rounded
        color="primary"
        :indeterminate="percent < 0"
        :value="percent < 0 ? undefined : percent / 100"
      />
      <span class="mono">{{ lastLogLine }}</span>
    </div>
    <p v-if="status === 'failed' && agent.lastProbeError" class="tool-card__error" role="note">
      {{ agent.lastProbeError }}
    </p>

    <footer class="tool-card__actions">
      <q-btn
        v-for="a in actions"
        :key="a.key"
        v-bind="BTN"
        :color="a.color ?? (a.key === 'remove' ? 'negative' : undefined)"
        :icon="a.icon"
        :label="a.label ?? t(`cap.coding.tool.actions.${a.key}`)"
        :title="a.title ?? t(`cap.coding.tool.actions.${a.key}`)"
        :aria-label="`${a.label ?? t(`cap.coding.tool.actions.${a.key}`)} ${title}`"
        :loading="a.loading"
        :disable="a.disable"
        @click="fire(a.key)"
      />
    </footer>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { CodingAgent } from '../api';
import type { AgentPreset } from '../../fleet/api';
import type { LoginKind } from '../../fleet/api';
import { installPercent, loginKind, toolIcon } from '../../fleet/api';

const props = defineProps<{ agent: CodingAgent; preset?: AgentPreset; busy?: string }>();
const emit = defineEmits<{
  toggle: [enabled: boolean];
  login: [];
  test: [];
  probe: [];
  update: [];
  edit: [];
  remove: [];
}>();
const { t, te } = useI18n();

// 模型 · 思考强度（只设了强度时模型写「默认模型」）
const modelText = computed(() => {
  const p = props.preset;
  if (!p?.model && !p?.effort) return '';
  const e =
    p.effort && te(`cap.coding.tool.effortLevel.${p.effort}`) ? t(`cap.coding.tool.effortLevel.${p.effort}`) : p.effort;
  return [p.model || t('cap.coding.tool.edit.modelDefault'), e].filter(Boolean).join(' · ');
});

const BTN = { flat: true, dense: true, noCaps: true, size: 'sm' } as const;
const DOT: Record<string, string> = {
  disabled: 'cap-dot--off',
  ok: 'cap-dot--ok',
  failed: 'cap-dot--alert',
  needLogin: 'cap-dot--watch',
  never: 'cap-dot--pending',
};

const isFleet = computed(() => (props.agent.command ?? '').startsWith('fleet:'));
// 运行台接入的工具以运行台清单的名称为准（不带 CLI）；自定义的用登记名
const title = computed(() =>
  props.preset ? props.preset.label : props.agent.displayName || props.agent.agentKey || '',
);
const icon = computed(() => toolIcon(props.preset?.id ?? '') || props.agent.iconUrl || '');

const status = computed(() => {
  if (!props.agent.enabled) return 'disabled';
  if (!props.agent.lastProbeOk && props.agent.lastProbeError) return 'failed';
  if (props.preset?.readiness === 'need_login') return 'needLogin';
  if (props.agent.lastProbeOk) return 'ok';
  return 'never';
});

// 权限：运行台接入的看运行台档位；ACP 的看登记的默认自治级别
const MODE_ICON: Record<string, string> = { manual: 'verified_user', auto_edit: 'edit_note', full: 'lock_open' };
const AUTONOMY_MODE: Record<string, string> = { readonly: 'manual', workspace: 'auto_edit', full: 'full' };
const mode = computed(() =>
  isFleet.value ? props.preset?.mode || 'manual' : AUTONOMY_MODE[props.agent.defaultAutonomy || 'readonly'] || 'manual',
);
const permIcon = computed(() => MODE_ICON[mode.value] ?? 'verified_user');
const permLabel = computed(() => t(`cap.coding.modes.${mode.value}`));
const permTone = computed(() => (mode.value === 'full' ? 'alert' : mode.value === 'auto_edit' ? 'watch' : ''));

// 一行按钮：登录（装好的运行台工具）· 测试 · 探测 · 更新（可安装的）· 编辑 · 删除
type ActionKey = 'login' | 'test' | 'probe' | 'update' | 'edit' | 'remove';
type Action = {
  key: ActionKey;
  icon: string;
  loading?: boolean;
  disable?: boolean;
  label?: string;
  title?: string;
  color?: string;
};
const installing = computed(() => props.preset?.installState === 'running');
const percent = computed(() => installPercent(props.preset?.installLog ?? ''));
const lastLogLine = computed(() => (props.preset?.installLog ?? '').trim().split('\n').pop()?.trim() ?? '');
// 更新按钮四种样子：检查中（转圈）/ 更新中（进度）/ 有新版（醒目）/ 已是最新或没查过（点了先查）
const updateAction = computed<Action>(() => {
  const p = props.preset as AgentPreset;
  const local = shortVersion.value || t('cap.coding.tool.noVersion');
  if (installing.value)
    return {
      key: 'update',
      icon: 'downloading',
      disable: true,
      label:
        percent.value < 0
          ? t('cap.coding.tool.update.running')
          : t('cap.coding.tool.update.runningPct', { pct: percent.value }),
    };
  if (props.busy === 'update') return { key: 'update', icon: 'system_update_alt', loading: true };
  if (p.updateAvailable)
    return {
      key: 'update',
      icon: 'upgrade',
      color: 'warning',
      label: t('cap.coding.tool.update.available'),
      title: t('cap.coding.tool.update.tipAvailable', { local, remote: p.latestVersion }),
    };
  return {
    key: 'update',
    icon: 'system_update_alt',
    title: p.latestVersion
      ? t('cap.coding.tool.update.tipLatest', { local, remote: p.latestVersion })
      : t('cap.coding.tool.update.tipUnchecked', { local }),
  };
});
const actions = computed(() => {
  const p = props.preset;
  const list: Action[] = [];
  if (p?.available) list.push({ key: 'login', icon: 'login' });
  list.push({ key: 'test', icon: 'science' }, { key: 'probe', icon: 'network_check', loading: props.busy === 'probe' });
  if (p?.install) list.push(updateAction.value);
  list.push({ key: 'edit', icon: 'edit' }, { key: 'remove', icon: 'delete' });
  return list;
});
// 这几个事件都不带参数；defineEmits 的重载签名不接受联合键，收窄成单一签名
const fire = (key: ActionKey) => (emit as (e: ActionKey) => void)(key);

const LOGIN_ICON: Record<LoginKind, string> = {
  account: 'how_to_reg',
  key: 'vpn_key',
  in: 'how_to_reg',
  out: 'person_off',
  unknown: 'help_outline',
};
// 登录状态：运行台接入的工具才有（自定义 ACP 工具的登录不归这里管）
const login = computed(() => {
  const p = props.preset;
  if (!p) return null;
  const kind = loginKind(p);
  const key = p.keyName ? t('cap.coding.tool.login.token') : t('cap.coding.tool.login.key');
  return { kind, text: t(`cap.coding.tool.loginState.${kind}`, { account: p.account, hint: p.keyHint, key }) };
});

const shortVersion = computed(() => (props.preset?.version ?? '').replace(/\s*\(.*\)\s*$/, ''));
const updatedAt = computed(() => {
  const iso = props.preset?.toolUpdatedAt;
  if (!iso) return '';
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString();
});
</script>

<style scoped lang="sass">
.tool-card
  container-type: inline-size
  display: flex
  flex-direction: column
  gap: var(--nt-space-2)
  padding: var(--nt-space-3)
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-l)
  background: var(--nt-tile-bg, var(--nt-bg-2))
  &.is-off
    opacity: 0.72
.tool-card__head
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  min-width: 0
.tool-card__logo
  flex: none
  width: 32px
  height: 32px
  border-radius: var(--nt-radius-m)
  object-fit: cover
.tool-card__logo--letter
  display: inline-flex
  align-items: center
  justify-content: center
  background: var(--nt-accent-soft)
  color: var(--nt-accent)
  font-weight: 600
.tool-card__name
  flex: 1
  min-width: 0
  margin: 0
  overflow: hidden
  color: var(--nt-text-1)
  font-size: var(--nt-fs-body)
  font-weight: 600
  text-overflow: ellipsis
  white-space: nowrap
.tool-card__status
  display: inline-flex
  flex: none
  align-items: center
  gap: var(--nt-space-1)
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
  &.is-failed
    color: var(--nt-danger)
  &.is-needLogin
    color: var(--nt-warn)
  &.is-ok
    color: var(--nt-success)
.tool-card__facts
  display: flex
  flex-wrap: wrap
  gap: var(--nt-space-1) var(--nt-space-3)
  margin: 0
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
  span
    display: inline-flex
    align-items: center
    gap: var(--nt-space-1)
  .is-login-account, .is-login-key, .is-login-in
    color: var(--nt-success)
  .is-login-out
    color: var(--nt-warn)
  .is-watch
    color: var(--nt-warn)
  .is-alert
    color: var(--nt-danger)
.tool-card__progress
  display: flex
  flex-direction: column
  gap: var(--nt-space-1)
  span
    overflow: hidden
    color: var(--nt-text-2)
    font-size: var(--nt-fs-micro)
    text-overflow: ellipsis
    white-space: nowrap
.tool-card__error
  display: -webkit-box
  margin: 0
  overflow: hidden
  color: var(--nt-danger)
  font-size: var(--nt-fs-caption)
  -webkit-box-orient: vertical
  -webkit-line-clamp: 2
.tool-card__actions
  display: flex
  flex-wrap: wrap
  justify-content: flex-end
  gap: var(--nt-space-1)
  margin-top: auto
  padding-top: var(--nt-space-2)
  border-top: 1px solid var(--nt-border-1)
// 卡片窄时（手机、窄栏）按钮只留图标，仍保持一行；名称靠 title / aria-label
@container (max-width: 419px)
  // Quasar 的 .block 工具类自带 !important，这里要同级才能盖住
  .tool-card__actions :deep(.q-btn .block)
    display: none !important
.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums
</style>
