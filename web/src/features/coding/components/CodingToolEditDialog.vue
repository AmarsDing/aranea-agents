<template>
  <AppDialog
    v-model="open"
    size="md"
    :loading="saving"
    :title="t('cap.coding.tool.edit.title', { name: preset?.label ?? '' })"
  >
    <template #header-prefix>
      <img v-if="preset && toolIcon(preset.id)" :src="toolIcon(preset.id)" class="tool-edit__logo" alt="" />
    </template>

    <form v-if="agent && preset" class="tool-edit" @submit.prevent="save">
      <section>
        <h3>{{ t('cap.coding.tool.edit.basic') }}</h3>
        <q-input v-model="form.displayName" dense outlined :label="t('cap.coding.tool.edit.displayName')" />
        <q-toggle v-model="form.enabled" color="primary" :label="t('cap.coding.tool.edit.enabled')" />
      </section>

      <section>
        <h3>{{ t('cap.coding.tool.edit.run') }}</h3>
        <div class="tool-edit__modes" role="radiogroup" :aria-label="t('cap.coding.tool.edit.run')">
          <label
            v-for="m in MODES"
            :key="m"
            class="tool-edit__mode"
            :class="{ 'is-picked': form.mode === m, 'is-off': !supported(m) }"
          >
            <q-radio v-model="form.mode" dense :val="m" :disable="!supported(m)" />
            <q-icon :name="MODE_ICON[m]" size="18px" aria-hidden="true" />
            <span class="tool-edit__mode-text">
              <strong>{{ t(`cap.coding.modes.${m}`) }}</strong>
              <small>{{
                supported(m) ? t(`cap.coding.tool.edit.permHint.${m}`) : t('cap.coding.tool.edit.permUnsupported')
              }}</small>
            </span>
          </label>
        </div>
        <CodingToolModelPicker
          v-if="preset.supportsModel && open"
          v-model:model="form.model"
          v-model:effort="form.effort"
          :preset="preset"
        />
        <q-input
          v-model="form.args"
          dense
          outlined
          class="mono"
          :label="t('cap.coding.tool.edit.args')"
          :hint="t('cap.coding.tool.edit.argsHint')"
        />
        <q-toggle
          v-if="preset.install"
          v-model="form.autoUpdate"
          color="primary"
          :label="t('cap.coding.tool.edit.autoUpdate')"
        />
      </section>

      <section>
        <h3>{{ t('cap.coding.tool.edit.account') }}</h3>
        <div class="tool-edit__row">
          <span>{{ t('cap.coding.tool.edit.loginState', { state: loginState }) }}</span>
          <q-btn flat dense no-caps icon="login" :label="t('cap.coding.tool.actions.login')" @click="emit('login')" />
        </div>
        <p v-if="preset.keyHint" class="tool-edit__note mono">
          {{ t('cap.coding.tool.login.keySaved', { hint: preset.keyHint, key: keyWord }) }}
        </p>
      </section>

      <section>
        <h3>{{ t('cap.coding.tool.edit.advanced') }}</h3>
        <p class="tool-edit__note mono">
          {{ t('cap.coding.tool.edit.access', { cmd: preset.resolvedPath || preset.command }) }}
        </p>
        <p class="tool-edit__note mono">{{ t('cap.coding.tool.edit.key', { key: agent.agentKey }) }}</p>
      </section>

      <p v-if="error" class="tool-edit__error" role="alert">{{ error }}</p>
    </form>

    <template #actions>
      <q-btn flat rounded no-caps :label="t('cap.coding.form.cancel')" @click="open = false" />
      <q-btn
        color="primary"
        rounded
        unelevated
        no-caps
        :loading="saving"
        :label="t('cap.coding.form.save')"
        @click="save"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import AppDialog from '../../../components/common/AppDialog.vue';
import type { CodingAgent } from '../api';
import { upsertCodingAgent } from '../api';
import type { AgentPreset, PermissionMode } from '../../fleet/api';
import { errText, loginKind, toolIcon, updateAgentSetting } from '../../fleet/api';
import CodingToolModelPicker from './CodingToolModelPicker.vue';

// 运行台接入的工具的编辑页：只放用户真会改的四组，命令、协议等由运行台清单决定
const open = defineModel<boolean>({ default: false });
const props = defineProps<{ agent: CodingAgent | null; preset?: AgentPreset }>();
const emit = defineEmits<{ saved: []; login: [] }>();
const { t } = useI18n();

const MODES: PermissionMode[] = ['manual', 'auto_edit', 'full'];
const MODE_ICON: Record<PermissionMode, string> = {
  manual: 'verified_user',
  auto_edit: 'edit_note',
  full: 'lock_open',
};

const form = reactive({
  displayName: '',
  enabled: true,
  mode: 'manual' as PermissionMode,
  args: '',
  autoUpdate: false,
  model: '',
  effort: '',
});
const saving = ref(false);
const error = ref('');

const supported = (m: PermissionMode) =>
  m === 'manual' || (m === 'auto_edit' ? !!props.preset?.autoEditFlag : !!props.preset?.supportsFullAccess);
const keyWord = computed(() =>
  props.preset?.keyName ? t('cap.coding.tool.login.token') : t('cap.coding.tool.login.key'),
);
const loginState = computed(() =>
  props.preset
    ? t(`cap.coding.tool.loginState.${loginKind(props.preset)}`, {
        account: props.preset.account,
        hint: props.preset.keyHint,
        key: keyWord.value,
      })
    : '',
);

watch(open, (v) => {
  if (!v || !props.agent || !props.preset) return;
  error.value = '';
  Object.assign(form, {
    displayName: props.agent.displayName || props.preset.label,
    enabled: !!props.agent.enabled,
    mode: props.preset.mode || 'manual',
    args: props.preset.args.join(' '),
    autoUpdate: props.preset.autoUpdate,
    model: props.preset.model,
    effort: props.preset.effort,
  });
});

async function save() {
  const a = props.agent;
  const p = props.preset;
  if (!a || !p) return;
  saving.value = true;
  error.value = '';
  try {
    // env / args 省略 = 后端保留旧值（列表里的密钥是掩码，不能回传）
    await upsertCodingAgent({
      agentKey: a.agentKey,
      displayName: form.displayName.trim() || p.label,
      command: a.command,
      args: undefined,
      env: undefined,
      enabled: form.enabled,
      livenessOverrideJson: undefined,
      iconUrl: a.iconUrl,
      capabilities: a.capabilities,
      defaultAutonomy: a.defaultAutonomy,
    });
    const args = form.args.trim() ? form.args.trim().split(/\s+/) : [];
    await updateAgentSetting(p.id, form.mode, args, form.autoUpdate, form.model.trim(), form.effort);
    emit('saved');
    open.value = false;
  } catch (e) {
    error.value = errText(e);
  } finally {
    saving.value = false;
  }
}
</script>

<style scoped lang="sass">
.tool-edit
  display: flex
  flex-direction: column
  gap: var(--nt-space-4)
  section
    display: flex
    flex-direction: column
    gap: var(--nt-space-2)
  h3
    margin: 0
    color: var(--nt-text-2)
    font-size: var(--nt-fs-caption)
    font-weight: 600
.tool-edit__logo
  width: 28px
  height: 28px
  border-radius: var(--nt-radius-m)
  object-fit: cover
.tool-edit__modes
  display: flex
  flex-direction: column
  gap: var(--nt-space-1)
.tool-edit__mode
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  min-height: 44px
  padding: var(--nt-space-2) var(--nt-space-3)
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-m)
  cursor: pointer
  &.is-picked
    border-color: var(--nt-accent)
  &.is-off
    cursor: not-allowed
    opacity: 0.6
  &:focus-within
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
.tool-edit__mode-text
  display: flex
  flex-direction: column
  min-width: 0
  strong
    color: var(--nt-text-1)
    font-size: var(--nt-fs-body)
  small
    color: var(--nt-text-2)
    font-size: var(--nt-fs-caption)
.tool-edit__row
  display: flex
  align-items: center
  justify-content: space-between
  gap: var(--nt-space-2)
  color: var(--nt-text-1)
.tool-edit__note
  margin: 0
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
  overflow-wrap: anywhere
.tool-edit__error
  margin: 0
  color: var(--nt-danger)
  font-size: var(--nt-fs-caption)
.mono
  font-family: var(--nt-font-mono)
</style>
