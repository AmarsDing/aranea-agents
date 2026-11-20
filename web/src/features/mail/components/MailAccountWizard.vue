<template>
  <!-- 添加邮箱向导（09-邮件.md §3.2）：填地址 → 填授权码 → 测试连接并开始同步。编辑已有账号时是一张表。 -->
  <AppDialog
    v-model="open"
    persistent
    size="md"
    position="standard"
    :title="editing ? t('mailPage.wizEditTitle') : t('mailPage.wizTitle')"
    :subtitle="editing ? '' : t(`mailPage.wizStep${step}`)"
  >
    <div class="ml-wiz">
      <div v-if="!editing" class="ml-wiz__steps" aria-hidden="true">
        <span v-for="n in 3" :key="n" :class="{ 'ml-wiz__steps--on': n <= step }" />
      </div>
      <q-banner v-if="error" rounded class="app-page-error-banner" role="alert">{{ error }}</q-banner>

      <!-- 第 1 步：邮箱地址 -->
      <template v-if="editing || step === 1">
        <q-input
          v-model="form.email"
          outlined
          dense
          autofocus
          type="email"
          :label="t('mailPage.wizEmail')"
          :loading="detecting"
          @keydown.enter.prevent="!editing && next()"
        />
        <div
          v-if="!editing && provider?.found"
          class="ml-wiz__prov"
          :class="{ 'ml-wiz__prov--warn': !provider.supported }"
          role="status"
        >
          <q-icon :name="provider.supported ? 'check_circle' : 'info'" size="18px" aria-hidden="true" />
          <span v-if="provider.supported">
            {{ t('mailPage.wizDetected', { name: provider.name, host: provider.imap_host }) }}
          </span>
          <span v-else>{{ t('mailPage.wizUnsupported', { name: provider.name }) }}</span>
        </div>
        <p v-else-if="!editing && detected && !provider?.found" class="ml-hint">{{ t('mailPage.wizNotDetected') }}</p>
        <p v-if="!editing" class="ml-hint">
          {{ t('mailPage.wizSupportedList') }}
          <a href="#" class="ml-link" @click.prevent="manual = !manual">{{ t('mailPage.wizManual') }}</a>
        </p>
        <div v-if="editing || manual" class="ml-wiz__servers">
          <q-select
            v-if="!editing"
            v-model="presetKey"
            outlined
            dense
            emit-value
            map-options
            clearable
            :options="presetOptions"
            :label="t('mailPage.wizProvider')"
            @update:model-value="applyPreset"
          />
          <div class="ml-wiz__grid">
            <q-input v-model="form.host" outlined dense :label="t('mailPage.wizImapHost')" />
            <q-input v-model.number="form.port" outlined dense type="number" :label="t('mailPage.wizPort')" />
            <q-select
              v-model="form.tls_mode"
              outlined
              dense
              emit-value
              map-options
              :options="tlsOptions"
              :label="t('mailPage.wizTls')"
            />
          </div>
          <div class="ml-wiz__grid">
            <q-input v-model="form.smtp_host" outlined dense :label="t('mailPage.wizSmtpHost')" />
            <q-input v-model.number="form.smtp_port" outlined dense type="number" :label="t('mailPage.wizPort')" />
            <q-select
              v-model="form.smtp_tls"
              outlined
              dense
              emit-value
              map-options
              :options="tlsOptions"
              :label="t('mailPage.wizTls')"
            />
          </div>
        </div>
      </template>

      <!-- 第 2 步：授权码 -->
      <template v-if="editing || step === 2">
        <q-input v-model="form.username" outlined dense :label="t('mailPage.wizUsername')" />
        <q-input
          v-model="form.password"
          outlined
          dense
          :type="showPassword ? 'text' : 'password'"
          autocomplete="new-password"
          :autofocus="!editing"
          :label="editing ? t('mailPage.wizPasswordKeep') : t('mailPage.wizPassword')"
          @keydown.enter.prevent="!editing && next()"
        >
          <template #append>
            <q-btn
              flat
              round
              dense
              :icon="showPassword ? 'visibility_off' : 'visibility'"
              :aria-label="showPassword ? t('mailPage.wizHidePassword') : t('mailPage.wizShowPassword')"
              :aria-pressed="showPassword"
              @click.stop="showPassword = !showPassword"
            />
          </template>
        </q-input>
        <div v-if="!editing" class="ml-wiz__help">
          <b>{{ t('mailPage.wizHowTo') }}</b>
          {{ helpText }}
          <p class="ml-hint">{{ t('mailPage.wizStoredHint') }}</p>
        </div>
      </template>

      <!-- 第 3 步：测试连接 + 同步范围 + 显示名 -->
      <template v-if="!editing && step === 3">
        <div class="ml-wiz__checks" aria-live="polite">
          <div class="ml-wiz__check" :class="checkClass(result?.imap_ok, result?.imap_error)">
            <q-icon :name="checkIcon(result?.imap_ok, result?.imap_error)" size="18px" aria-hidden="true" />
            <span>{{ t('mailPage.wizImapCheck') }}</span>
            <code class="ml-num">{{ form.host }}:{{ form.port }}</code>
            <span v-if="result?.imap_error" class="ml-wiz__why">{{ t(`mailPage.err.${result.imap_error}`) }}</span>
          </div>
          <div class="ml-wiz__check" :class="checkClass(result?.smtp_ok, result?.smtp_error, result?.smtp_skipped)">
            <q-icon
              :name="checkIcon(result?.smtp_ok, result?.smtp_error, result?.smtp_skipped)"
              size="18px"
              aria-hidden="true"
            />
            <span>{{ t('mailPage.wizSmtpCheck') }}</span>
            <code v-if="form.smtp_host" class="ml-num">{{ form.smtp_host }}:{{ form.smtp_port }}</code>
            <span v-if="result?.smtp_skipped" class="ml-wiz__why">{{ t('mailPage.wizSmtpSkipped') }}</span>
            <span v-else-if="result?.smtp_error" class="ml-wiz__why">{{ t(`mailPage.err.${result.smtp_error}`) }}</span>
          </div>
          <div v-if="authFailed" class="ml-wiz__help">
            <b>{{ t('mailPage.wizHowTo') }}</b>
            {{ helpText }}
          </div>
          <div v-if="result?.imap_ok" class="ml-wiz__check ml-wiz__check--ok">
            <q-icon name="inbox" size="18px" aria-hidden="true" />
            <span>{{ t('mailPage.wizInboxCount', { n: result.inbox_count }) }}</span>
          </div>
          <q-btn
            flat
            dense
            no-caps
            icon="refresh"
            :label="t('mailPage.wizRetest')"
            :loading="testing"
            @click="runTest"
          />
        </div>
        <div>
          <div class="ml-wiz__lab">{{ t('mailPage.wizRange') }}</div>
          <q-btn-toggle
            v-model="range"
            dense
            no-caps
            unelevated
            toggle-color="primary"
            :options="rangeOptions"
            :aria-label="t('mailPage.wizRange')"
          />
        </div>
        <q-input v-model="form.name" outlined dense :label="t('mailPage.wizName')" />
      </template>

      <template v-if="editing">
        <q-input v-model="form.name" outlined dense :label="t('mailPage.wizName')" />
      </template>
    </div>

    <template #actions>
      <q-btn flat no-caps :label="t('mailPage.cancel')" @click="open = false" />
      <template v-if="editing">
        <q-btn unelevated no-caps color="primary" :label="t('mailPage.save')" :loading="saving" @click="saveEdit" />
      </template>
      <template v-else>
        <q-btn v-if="step > 1" flat no-caps :label="t('mailPage.wizBack')" @click="back" />
        <q-btn
          unelevated
          no-caps
          color="primary"
          :label="step < 3 ? t('mailPage.wizNext') : t('mailPage.wizFinish')"
          :loading="detecting || saving"
          :disable="!canNext"
          @click="next"
        />
      </template>
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import AppDialog from '../../../components/common/AppDialog.vue';
import {
  createMailAccount,
  detectMailProvider,
  listMailProviders,
  testMailAccount,
  updateMailAccount,
  type MailAccountInput,
} from '../../office/api';
import type { MailAccountRow, MailProviderRow, MailTestResult } from '../../office/types';
import { syncSinceFor } from '../model';

const open = defineModel<boolean>({ required: true });
const props = defineProps<{ account?: MailAccountRow | null }>();
const emit = defineEmits<{ saved: [account: MailAccountRow, isNew: boolean] }>();

const { t, te } = useI18n();

const editing = computed(() => !!props.account);
const step = ref(1);
const manual = ref(false);
const detecting = ref(false);
const detected = ref(false);
const testing = ref(false);
const saving = ref(false);
const error = ref('');
const provider = ref<MailProviderRow | null>(null);
const presets = ref<MailProviderRow[]>([]);
const presetKey = ref<string | null>(null);
const result = ref<MailTestResult | null>(null);
const range = ref<'7d' | '30d' | '1y' | 'all'>('30d');
const showPassword = ref(false);
let lastDetected = '';

type Form = Required<Omit<MailAccountInput, 'sync_since'>>;
const blank = (): Form => ({
  name: '',
  host: '',
  port: 993,
  tls_mode: 'TLS_MODE_TLS',
  username: '',
  password: '',
  email: '',
  provider: '',
  smtp_host: '',
  smtp_port: 465,
  smtp_tls: 'TLS_MODE_TLS',
});
const form = ref<Form>(blank());

const tlsOptions = computed(() => [
  { value: 'TLS_MODE_TLS', label: 'SSL / TLS' },
  { value: 'TLS_MODE_STARTTLS', label: 'STARTTLS' },
  { value: 'TLS_MODE_NONE', label: t('mailPage.wizNoTls') },
]);
const rangeOptions = computed(() =>
  (['7d', '30d', '1y', 'all'] as const).map((v) => ({ value: v, label: t(`mailPage.range.${v}`) })),
);
const presetOptions = computed(() =>
  presets.value.map((p) => ({ value: p.key, label: p.name, disable: !p.supported })),
);

watch(open, (v) => {
  if (!v) return;
  error.value = '';
  result.value = null;
  step.value = 1;
  manual.value = false;
  detected.value = false;
  provider.value = null;
  presetKey.value = null;
  range.value = '30d';
  showPassword.value = false;
  const a = props.account;
  form.value = a
    ? {
        name: a.name,
        host: a.host,
        port: a.port,
        tls_mode: a.tls_mode,
        username: a.username,
        password: '',
        email: a.email,
        provider: a.provider,
        smtp_host: a.smtp_host,
        smtp_port: a.smtp_port || 465,
        smtp_tls: a.smtp_tls === 'TLS_MODE_UNSPECIFIED' ? 'TLS_MODE_TLS' : a.smtp_tls,
      }
    : blank();
  if (!presets.value.length)
    void listMailProviders()
      .then((list) => (presets.value = list))
      .catch(() => undefined);
});

function applyPreset(key: string | null) {
  const p = presets.value.find((x) => x.key === key);
  if (!p) return;
  provider.value = { ...p, found: true, detected_by: '' };
  fill(p);
}

function fill(p: MailProviderRow) {
  form.value.provider = p.key;
  form.value.host = p.imap_host;
  form.value.port = p.imap_port;
  form.value.tls_mode = p.imap_tls;
  form.value.smtp_host = p.smtp_host;
  form.value.smtp_port = p.smtp_port;
  form.value.smtp_tls = p.smtp_tls;
  if (!form.value.name) form.value.name = p.name;
}

const helpText = computed(() => {
  const key = `mailPage.help.${form.value.provider || 'generic'}`;
  return te(key) ? t(key) : t('mailPage.help.generic');
});

const emailOk = computed(() => /^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(form.value.email.trim()));
const canNext = computed(() => {
  if (step.value === 1) return emailOk.value && (!provider.value?.found || provider.value.supported || manual.value);
  if (step.value === 2) return !!form.value.host && !!form.value.username && !!form.value.password;
  return !!result.value?.imap_ok && !testing.value;
});

/** 按地址认服务商；认出来就把服务器填好，认不出就展开手动填。 */
async function detect() {
  const email = form.value.email.trim();
  detecting.value = true;
  try {
    const p = await detectMailProvider(email);
    if (form.value.email.trim() !== email) return; // 等结果时又改了地址
    provider.value = p;
    if (!form.value.username || form.value.username === lastDetected) form.value.username = email;
    lastDetected = email;
    detected.value = true;
    if (p.found) {
      presetKey.value = p.key;
      fill(p);
    } else {
      // 上一个地址认出来的服务器不能带给这个地址
      if (form.value.provider) Object.assign(form.value, { provider: '', host: '', smtp_host: '' });
      presetKey.value = null;
      manual.value = true;
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    detecting.value = false;
  }
}

// 边输边认（停顿 600ms），第 1 步就能看到「认出来了：…」
let detectTimer: ReturnType<typeof setTimeout> | undefined;
watch(
  () => form.value.email,
  () => {
    clearTimeout(detectTimer);
    if (editing.value || step.value !== 1 || !emailOk.value) return;
    detectTimer = setTimeout(() => void detect(), 600);
  },
);

async function next() {
  error.value = '';
  if (step.value === 1) {
    if (!emailOk.value) return;
    if (form.value.email.trim() !== lastDetected) {
      // 刚认出来：先停在这一步让人看到结果，再点一次下一步
      await detect();
      return;
    }
    // 认出来了但要 OAuth、或者认不出还没填服务器：停在这一步
    if (provider.value?.found && !provider.value.supported && !manual.value) return;
    if (!form.value.host) {
      manual.value = true;
      return;
    }
    step.value = 2;
    return;
  }
  if (step.value === 2) {
    step.value = 3;
    void runTest();
    return;
  }
  await finish();
}

// 回上一步改了再来：上一次的失败结论作废，不能留在页面上
function back() {
  error.value = '';
  result.value = null;
  step.value--;
}

const authFailed = computed(() => !testing.value && result.value?.imap_error === 'auth');

async function runTest() {
  testing.value = true;
  error.value = '';
  result.value = null;
  try {
    result.value = await testMailAccount({ ...form.value, email: form.value.email.trim() });
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    testing.value = false;
  }
}

async function finish() {
  saving.value = true;
  try {
    const acc = await createMailAccount({
      ...form.value,
      email: form.value.email.trim(),
      name: form.value.name.trim() || form.value.email.trim(),
      sync_since: syncSinceFor(range.value, new Date()),
    });
    emit('saved', acc, true);
    open.value = false;
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    saving.value = false;
  }
}

async function saveEdit() {
  if (!props.account) return;
  saving.value = true;
  error.value = '';
  try {
    const acc = await updateMailAccount(props.account.id, { ...form.value, email: form.value.email.trim() });
    emit('saved', acc, false);
    open.value = false;
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    saving.value = false;
  }
}

function checkIcon(ok?: boolean, err?: string, skipped?: boolean): string {
  if (testing.value) return 'hourglass_empty';
  if (skipped) return 'remove_circle_outline';
  if (ok) return 'check_circle';
  return err ? 'error' : 'radio_button_unchecked';
}

function checkClass(ok?: boolean, err?: string, skipped?: boolean): string {
  if (testing.value || skipped) return '';
  if (ok) return 'ml-wiz__check--ok';
  return err ? 'ml-wiz__check--bad' : '';
}
</script>
