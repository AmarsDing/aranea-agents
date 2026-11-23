<template>
  <AppDialog v-model="open" size="lg" position="standard" :title="t('cap.coding.tool.login.title', { name })">
    <template #header-prefix>
      <img v-if="preset && toolIcon(preset.id)" :src="toolIcon(preset.id)" class="tool-login__logo" alt="" />
    </template>

    <div v-if="preset" class="tool-login">
      <p class="tool-login__state" :class="{ 'is-ok': signedIn }" role="status">
        <q-icon :name="signedIn ? 'verified' : 'person_off'" size="18px" aria-hidden="true" />{{ stateText }}
      </p>

      <p v-if="preset.keyOnly" class="tool-login__hint">{{ t('cap.coding.tool.login.keyOnly', { name }) }}</p>

      <q-tabs
        v-if="preset.keyEnv && !preset.keyOnly"
        v-model="tab"
        dense
        no-caps
        align="left"
        narrow-indicator
        active-color="primary"
        indicator-color="primary"
        class="tool-login__tabs"
      >
        <q-tab name="account" icon="account_circle" :label="t('cap.coding.tool.login.account')" />
        <q-tab name="key" icon="vpn_key" :label="keyWord" />
      </q-tabs>

      <!-- 账号登录：跑工具自己的登录命令，把登录页 / 设备码 / 二维码摆出来，授权完自动确认 -->
      <section v-if="tab === 'account'" class="tool-login__pane">
        <template v-if="preset.hasLogin">
          <p v-if="te(`cap.coding.tool.login.note.${preset.id}`)" class="tool-login__hint">
            {{ t(`cap.coding.tool.login.note.${preset.id}`) }}
          </p>
          <div class="tool-login__row">
            <q-input
              v-if="preset.emailLogin"
              v-model="email"
              dense
              outlined
              type="email"
              autocomplete="email"
              class="tool-login__grow"
              :label="t('cap.coding.tool.login.email')"
              :disable="login.running"
            >
              <template #append>
                <q-icon name="help_outline" size="16px">
                  <q-tooltip>{{ t('cap.coding.tool.login.emailTip') }}</q-tooltip>
                </q-icon>
              </template>
            </q-input>
            <q-btn
              color="primary"
              unelevated
              rounded
              no-caps
              icon="login"
              :loading="starting"
              :disable="login.running"
              :label="signedIn ? t('cap.coding.tool.login.restart') : t('cap.coding.tool.login.start')"
              @click="startLogin"
            />
          </div>

          <ol v-if="started" class="tool-login__steps">
            <li :class="login.url ? 'is-ok' : 'is-run'">
              <q-spinner v-if="!login.url && login.running" size="16px" />
              <q-icon v-else name="open_in_browser" size="18px" aria-hidden="true" />
              <span class="tool-login__grow">{{
                login.url ? t('cap.coding.tool.login.pageOpened') : t('cap.coding.tool.login.starting')
              }}</span>
              <template v-if="login.url">
                <q-btn
                  flat
                  dense
                  no-caps
                  color="primary"
                  icon="open_in_new"
                  :label="t('cap.coding.tool.login.openPage')"
                  :href="login.url"
                  target="_blank"
                  rel="noopener"
                />
                <q-btn
                  flat
                  dense
                  round
                  icon="content_copy"
                  :aria-label="t('cap.coding.tool.login.copyLink')"
                  @click="copy(login.url)"
                >
                  <q-tooltip>{{ t('cap.coding.tool.login.copyLink') }}</q-tooltip>
                </q-btn>
              </template>
            </li>
            <li v-if="login.code" class="is-ok">
              <q-icon name="pin" size="18px" aria-hidden="true" />
              <span class="tool-login__grow">{{ t('cap.coding.tool.login.code') }}</span>
              <code class="tool-login__code">{{ login.code }}</code>
              <q-btn
                flat
                dense
                round
                icon="content_copy"
                :aria-label="t('cap.coding.tool.login.copyLink')"
                @click="copy(login.code)"
              />
            </li>
            <li v-if="login.qr" class="is-ok tool-login__qr-row">
              <q-icon name="qr_code_2" size="18px" aria-hidden="true" />
              <span class="tool-login__grow">{{ t('cap.coding.tool.login.scan') }}</span>
              <img :src="login.qr" class="tool-login__qr" :alt="t('cap.coding.tool.login.scan')" />
            </li>
            <li :class="login.done ? (login.ok ? 'is-ok' : 'is-fail') : 'is-run'" aria-live="polite">
              <q-spinner v-if="!login.done" size="16px" />
              <q-icon v-else :name="login.ok ? 'check_circle' : 'error'" size="18px" aria-hidden="true" />
              <span class="tool-login__grow">{{ resultText }}</span>
            </li>
          </ol>
        </template>

        <!-- 没有登录命令的：在工具自己的首次启动界面里登录，完成后检查一次 -->
        <template v-else>
          <p class="tool-login__hint">{{ t('cap.coding.tool.login.firstRun', { name, hint: preset.loginHint }) }}</p>
          <div class="tool-login__row">
            <q-btn
              v-if="!login.terminalId"
              color="primary"
              unelevated
              rounded
              no-caps
              icon="terminal"
              :loading="starting"
              :label="t('cap.coding.tool.login.openTool', { name })"
              @click="startLogin"
            />
            <q-btn
              v-if="preset.supportsTest"
              flat
              rounded
              no-caps
              icon="fact_check"
              :loading="checking"
              :label="t('cap.coding.tool.login.check')"
              @click="checkSignedIn"
            />
            <span v-if="checkResult" class="tool-login__grow" :class="checkOk ? 'is-ok' : 'is-fail'" role="status">{{
              checkResult
            }}</span>
          </div>
          <!-- 首次启动界面里出现登录链接时也摆出来 -->
          <div v-if="login.url" class="tool-login__row">
            <q-icon name="open_in_browser" size="18px" aria-hidden="true" />
            <span class="tool-login__grow">{{ t('cap.coding.tool.login.linkFound') }}</span>
            <q-btn
              flat
              dense
              no-caps
              color="primary"
              icon="open_in_new"
              :label="t('cap.coding.tool.login.openPage')"
              :href="login.url"
              target="_blank"
              rel="noopener"
            />
            <q-btn
              flat
              dense
              round
              icon="content_copy"
              :aria-label="t('cap.coding.tool.login.copyLink')"
              @click="copy(login.url)"
            />
          </div>
        </template>

        <q-expansion-item
          v-if="login.terminalId"
          v-model="showTerm"
          dense
          icon="terminal"
          :label="t('cap.coding.tool.login.terminal')"
          class="tool-login__term-box"
        >
          <div class="tool-login__term">
            <FleetTerminalView v-if="showTerm" :key="login.terminalId" :terminal-id="login.terminalId" :active="open" />
          </div>
        </q-expansion-item>
      </section>

      <!-- API Key：先真实验证，通过才加密保存到「连接 › 凭据」 -->
      <section v-else class="tool-login__pane">
        <div v-if="preset.keyHint" class="tool-login__row">
          <q-icon name="vpn_key" size="18px" class="tool-login__ok-icon" aria-hidden="true" />
          <span class="tool-login__grow mono">{{
            t('cap.coding.tool.login.keySaved', { hint: preset.keyHint, key: keyWord })
          }}</span>
          <span v-if="preset.host" class="mono">{{ t('cap.coding.tool.login.hostSaved', { host: preset.host }) }}</span>
          <q-btn
            flat
            dense
            round
            icon="delete"
            color="negative"
            :loading="removing"
            :aria-label="t('cap.coding.tool.login.keyRemove')"
            @click="removeKey"
          >
            <q-tooltip>{{ t('cap.coding.tool.login.keyRemove') }}</q-tooltip>
          </q-btn>
        </div>
        <form class="tool-login__row" @submit.prevent="verifyKey">
          <q-input
            v-model="key"
            dense
            outlined
            autocomplete="off"
            class="tool-login__grow mono"
            :type="showKey ? 'text' : 'password'"
            :label="keyWord"
            :disable="verifying"
          >
            <template #append>
              <q-icon
                :name="showKey ? 'visibility_off' : 'visibility'"
                class="cursor-pointer"
                size="18px"
                :aria-label="t('cap.coding.tool.login.showKey')"
                @click="showKey = !showKey"
              />
              <q-icon name="help_outline" size="16px">
                <q-tooltip max-width="280px">{{
                  t('cap.coding.tool.login.keyTip', { name, env: preset.keyEnv })
                }}</q-tooltip>
              </q-icon>
            </template>
          </q-input>
          <q-btn
            v-if="preset.keyUrl"
            flat
            dense
            round
            icon="open_in_new"
            :href="preset.keyUrl"
            target="_blank"
            rel="noopener"
            :aria-label="t('cap.coding.tool.login.getKey')"
          >
            <q-tooltip>{{ t('cap.coding.tool.login.getKey') }}</q-tooltip>
          </q-btn>
          <q-btn
            type="submit"
            color="primary"
            unelevated
            rounded
            no-caps
            icon="verified_user"
            :loading="verifying"
            :disable="!key.trim()"
            :label="t('cap.coding.tool.login.verify')"
          />
        </form>
        <!-- 企业自定义 API 域名（TRAE 的 TRAECLI_HOST），随令牌一起保存 -->
        <q-input
          v-if="preset.hostEnv"
          v-model="host"
          dense
          outlined
          type="url"
          inputmode="url"
          autocomplete="off"
          class="mono"
          placeholder="https://api.example.com"
          :label="t('cap.coding.tool.login.host')"
          :disable="verifying"
        >
          <template #append>
            <q-icon name="help_outline" size="16px">
              <q-tooltip max-width="280px">{{ t('cap.coding.tool.login.hostTip') }}</q-tooltip>
            </q-icon>
          </template>
        </q-input>
        <p v-if="keyResult" class="tool-login__result" :class="keyOk ? 'is-ok' : 'is-fail'" role="status">
          <q-icon :name="keyOk ? 'check_circle' : 'error'" size="18px" aria-hidden="true" />{{ keyResult }}
        </p>
        <!-- 验证过程原样给出：失败时一眼看到工具 / 接口说了什么 -->
        <q-expansion-item
          v-if="keyLog"
          v-model="showKeyLog"
          dense
          icon="receipt_long"
          :label="t('cap.coding.tool.login.keyLog')"
          class="tool-login__term-box"
        >
          <CodingToolConsole :title="t('cap.coding.tool.add.console')" :text="keyLog" />
        </q-expansion-item>
      </section>

      <p v-if="error" class="tool-login__result is-fail" role="alert">{{ error }}</p>
    </div>

    <template #actions>
      <q-btn
        color="primary"
        rounded
        unelevated
        no-caps
        :label="t('cap.coding.tool.login.done')"
        @click="open = false"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { copyToClipboard, useQuasar } from 'quasar';
import AppDialog from '../../../components/common/AppDialog.vue';
import FleetTerminalView from '../../fleet/components/FleetTerminalView.vue';
import CodingToolConsole from './CodingToolConsole.vue';
import type { AgentPreset, LoginKind, ToolLoginState } from '../../fleet/api';
import {
  closeTerminal,
  deleteToolKey,
  errText,
  getToolLogin,
  listAgents,
  loginKind,
  saveToolKey,
  startToolLogin,
  testAgent,
  toolIcon,
} from '../../fleet/api';

// aranea 不经手账号密码：账号登录跑工具自己的登录流程（浏览器授权 / 设备码 / 扫码），
// API Key 验证通过后加密存进「连接 › 凭据」，启动工具时注入
const open = defineModel<boolean>({ default: false });
const props = defineProps<{ preset?: AgentPreset }>();
const emit = defineEmits<{ changed: [] }>();
const { t, te } = useI18n();
const $q = useQuasar();

const EMPTY: ToolLoginState = {
  terminalId: '',
  running: false,
  done: false,
  ok: false,
  url: '',
  code: '',
  qr: '',
  account: '',
  message: '',
};
const tab = ref<'account' | 'key'>('account');
const login = reactive<ToolLoginState>({ ...EMPTY });
const started = ref(false);
const starting = ref(false);
const showTerm = ref(false);
const email = ref('');
const error = ref('');
const checking = ref(false);
const checkResult = ref('');
const checkOk = ref(false);
const key = ref('');
const host = ref('');
const keyWord = computed(() =>
  props.preset?.keyName ? t('cap.coding.tool.login.token') : t('cap.coding.tool.login.key'),
);
const showKey = ref(false);
const verifying = ref(false);
const removing = ref(false);
const keyResult = ref('');
const keyLog = ref('');
const showKeyLog = ref(false);
const keyOk = ref(false);
let timer: ReturnType<typeof setTimeout> | undefined;

const name = computed(() => props.preset?.label ?? '');
const account = computed(() => login.account || props.preset?.account || '');
// 与卡片同一套口径；本弹窗里刚登录成功的以这次结果为准
const kind = computed<LoginKind>(() => {
  if (login.ok || checkOk.value) return account.value ? 'account' : 'in';
  return props.preset ? loginKind(props.preset) : 'unknown';
});
const signedIn = computed(() => ['account', 'key', 'in'].includes(kind.value));
const stateText = computed(() =>
  t(`cap.coding.tool.loginState.${kind.value}`, {
    account: account.value,
    hint: props.preset?.keyHint ?? '',
    key: keyWord.value,
  }),
);
const resultText = computed(() => {
  if (!login.done) return t('cap.coding.tool.login.waiting');
  if (login.ok)
    return login.account
      ? t('cap.coding.tool.login.okAccount', { account: login.account })
      : t('cap.coding.tool.login.ok');
  return `${t('cap.coding.tool.login.failed')}${login.message ? `：${login.message}` : ''}`;
});

function reset() {
  clearTimeout(timer);
  Object.assign(login, EMPTY);
  started.value = starting.value = checking.value = verifying.value = false;
  error.value = checkResult.value = keyResult.value = keyLog.value = key.value = '';
  checkOk.value = keyOk.value = showKey.value = false;
  const p = props.preset;
  host.value = p?.host ?? '';
  tab.value = p?.keyOnly || (p?.keyEnv && !p.hasLogin) ? 'key' : 'account';
  showTerm.value = !props.preset?.hasLogin; // 没有登录命令的要在终端里操作，默认展开
}

watch(open, async (v) => {
  if (v) return reset();
  clearTimeout(timer);
  if (login.terminalId && !login.done) await closeTerminal(login.terminalId).catch(() => undefined);
  emit('changed');
});
onBeforeUnmount(() => clearTimeout(timer));

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

async function startLogin() {
  if (!props.preset) return;
  starting.value = true;
  error.value = '';
  try {
    Object.assign(login, EMPTY, await startToolLogin(props.preset.id, email.value.trim()));
    started.value = true;
    poll();
  } catch (e) {
    error.value = errText(e);
  } finally {
    starting.value = false;
  }
}

function poll() {
  clearTimeout(timer);
  timer = setTimeout(async () => {
    if (!open.value || !props.preset) return;
    try {
      Object.assign(login, await getToolLogin(props.preset.id));
      if (login.done) {
        showTerm.value = showTerm.value || !login.ok;
        if (login.ok) emit('changed');
        return;
      }
    } catch (e) {
      error.value = errText(e);
    }
    poll();
  }, 1200);
}

// 等后台连通测试出结论（检查登录、验证 Key 共用）
async function waitTest(id: string): Promise<AgentPreset | undefined> {
  for (;;) {
    await sleep(1200);
    const p = (await listAgents()).items.find((x) => x.id === id);
    if (!p || p.testState !== 'running') return p;
  }
}

async function checkSignedIn() {
  if (!props.preset) return;
  checking.value = true;
  checkResult.value = '';
  try {
    await testAgent(props.preset.id);
    const p = await waitTest(props.preset.id);
    checkOk.value = p?.testState === 'ok';
    checkResult.value = checkOk.value ? t('cap.coding.tool.login.ok') : (p?.testVerdict ?? '');
    if (checkOk.value) emit('changed');
  } catch (e) {
    checkResult.value = errText(e);
  } finally {
    checking.value = false;
  }
}

async function verifyKey() {
  if (!props.preset || !key.value.trim()) return;
  verifying.value = true;
  keyResult.value = keyLog.value = '';
  try {
    await saveToolKey(props.preset.id, key.value.trim(), host.value.trim());
    const p = await waitTest(props.preset.id);
    keyOk.value = p?.testState === 'ok';
    keyResult.value = p?.testVerdict ?? '';
    keyLog.value = p?.testLog ?? '';
    showKeyLog.value = !keyOk.value;
    if (keyOk.value) {
      key.value = '';
      emit('changed');
    }
  } catch (e) {
    keyOk.value = false;
    keyResult.value = errText(e);
  } finally {
    verifying.value = false;
  }
}

async function removeKey() {
  if (!props.preset) return;
  removing.value = true;
  try {
    await deleteToolKey(props.preset.id);
    keyOk.value = true; // 删除成功也是成功提示
    keyResult.value = t('cap.coding.tool.login.keyRemoved');
    emit('changed');
  } catch (e) {
    error.value = errText(e);
  } finally {
    removing.value = false;
  }
}

async function copy(text: string) {
  await copyToClipboard(text);
  $q.notify({ type: 'positive', message: t('cap.coding.tool.login.copied'), timeout: 1500 });
}
</script>

<style scoped lang="sass">
.tool-login
  display: flex
  flex-direction: column
  gap: var(--nt-space-3)
.tool-login__logo
  width: 28px
  height: 28px
  border-radius: var(--nt-radius-m)
  object-fit: cover
.tool-login__state
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  margin: 0
  color: var(--nt-text-2)
  &.is-ok
    color: var(--nt-success)
.tool-login__tabs
  border-bottom: 1px solid var(--nt-border-1)
.tool-login__pane
  display: flex
  flex-direction: column
  gap: var(--nt-space-3)
.tool-login__row
  display: flex
  flex-wrap: wrap
  align-items: center
  gap: var(--nt-space-2)
.tool-login__grow
  flex: 1
  min-width: 0
.tool-login__steps
  display: flex
  flex-direction: column
  gap: var(--nt-space-2)
  margin: 0
  padding: 0
  list-style: none
  li
    display: flex
    flex-wrap: wrap
    align-items: center
    gap: var(--nt-space-2)
    min-height: 44px
    padding: var(--nt-space-2) var(--nt-space-3)
    border: 1px solid var(--nt-border-1)
    border-radius: var(--nt-radius-m)
    color: var(--nt-text-1)
  .is-run
    border-color: var(--nt-accent)
  .is-ok > .q-icon
    color: var(--nt-success)
  .is-fail
    border-color: var(--nt-danger)
    color: var(--nt-danger)
.tool-login__code
  padding: var(--nt-space-1) var(--nt-space-3)
  border-radius: var(--nt-radius-s)
  background: var(--nt-data-bg)
  color: var(--nt-text-0)
  font-family: var(--nt-font-mono)
  font-size: var(--nt-fs-title)
  letter-spacing: 0.12em
.tool-login__qr
  width: 160px
  height: 160px
  border-radius: var(--nt-radius-m)
  image-rendering: pixelated
.tool-login__hint
  max-width: 72ch
  margin: 0
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
.tool-login__term-box
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-m)
.tool-login__term
  height: min(50vh, 420px)
  min-height: 280px
  overflow: hidden
  background: var(--nt-data-bg)
.tool-login__result
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  margin: 0
  font-size: var(--nt-fs-caption)
.is-ok
  color: var(--nt-success)
.is-fail
  color: var(--nt-danger)
.tool-login__ok-icon
  color: var(--nt-success)
.mono
  font-family: var(--nt-font-mono)
</style>
