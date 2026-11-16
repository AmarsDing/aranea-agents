<template>
  <form class="today-command" @submit.prevent="onSubmit" @keydown.esc="onEscape">
    <div class="today-command__row">
      <q-input
        ref="inputEl"
        v-model="draft"
        dense
        outlined
        maxlength="2000"
        autocomplete="off"
        :placeholder="t('today.command.placeholder')"
        :aria-label="t('today.command.label')"
        :disable="busy"
      >
        <template #prepend>
          <q-icon name="bolt" aria-hidden="true" />
        </template>
        <template #append>
          <kbd class="today-command__key today-mono" aria-hidden="true">/</kbd>
        </template>
      </q-input>
      <q-btn
        type="submit"
        unelevated
        no-caps
        color="primary"
        :label="submitLabel"
        :loading="busy"
        :disable="!canSubmit"
      />
    </div>

    <div v-if="text" class="today-command__preview">
      <q-option-group
        v-model="intentModel"
        class="today-command__intent"
        type="radio"
        inline
        :options="intentOptions"
        :disable="busy"
        :aria-label="t('today.command.intentLabel')"
      />
      <q-select
        v-if="intent === 'delegate'"
        v-model="recipientModel"
        class="today-command__to"
        dense
        outlined
        emit-value
        map-options
        :options="recipientOptions"
        :label="t('today.command.to')"
        :disable="busy || recipients.length === 0"
      />
      <p class="today-command__hint" :class="{ 'today-command__hint--confirm': confirming }" aria-live="polite">
        {{ hint }}
        <button v-if="confirming" type="button" class="today-link" :disabled="busy" @click="confirming = false">
          {{ t('today.command.cancel') }}
        </button>
      </p>
    </div>

    <p v-if="error" class="today-note today-note--danger" role="alert">
      {{ t('today.command.failed', { message: error }) }}
      <router-link v-if="errorLink" class="today-link" :to="errorLink">{{
        t('today.command.openSession')
      }}</router-link>
    </p>
  </form>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { parseCommand, recipientRef, type CommandIntent, type Recipient } from '../../features/today/command';
import type { RecipientsStatus } from '../../features/today/useDelegation';

/** 执行一条指令；失败时返回错误文案（可带会话入口），成功返回空串。 */
export type CommandRequest =
  | { intent: 'task' | 'note'; text: string }
  | { intent: 'delegate'; text: string; recipient: Recipient };
export type CommandFailure = { message: string; link?: string };

const props = defineProps<{
  recipients: Recipient[];
  recipientsStatus: RecipientsStatus;
  run: (request: CommandRequest) => Promise<CommandFailure | null>;
}>();

const { t } = useI18n();

const inputEl = ref<{ focus: () => void } | null>(null);
const draft = ref('');
/** 用户在预览里手动改过的意图与对象；清空输入后回到自动判断 */
const manualIntent = ref<CommandIntent | null>(null);
const manualRecipient = ref<string | null>(null);
/** 交办需要二次确认：第一次提交只展示将派给谁，第二次才真的发出 */
const confirming = ref(false);
const busy = ref(false);
const error = ref('');
const errorLink = ref('');

const parsed = computed(() => parseCommand(draft.value, props.recipients));
const intent = computed<CommandIntent>(() => manualIntent.value ?? parsed.value.intent);
// 手动改成别的意图时，对象名与「让」字不再有意义地剥掉：交办前缀还原成整句。
const text = computed(() => {
  if (intent.value === parsed.value.intent || parsed.value.intent === 'note') return parsed.value.text;
  return draft.value.trim();
});
const recipient = computed(() => {
  const wanted = manualRecipient.value ?? parsed.value.recipientRef;
  return props.recipients.find((r) => recipientRef(r) === wanted) ?? null;
});

const intentModel = computed<CommandIntent>({
  get: () => intent.value,
  set: (value) => (manualIntent.value = value),
});
const recipientModel = computed<string>({
  get: () => (recipient.value ? recipientRef(recipient.value) : ''),
  set: (value) => (manualRecipient.value = value),
});

const intentOptions = computed(() =>
  (['delegate', 'task', 'note'] as const).map((value) => ({ value, label: t(`today.command.intent.${value}`) })),
);
const recipientOptions = computed(() =>
  props.recipients.map((r) => ({
    value: recipientRef(r),
    label: r.kind === 'team' ? t('today.command.teamOption', { name: r.name }) : r.name,
  })),
);

const canSubmit = computed(() => {
  if (!text.value || busy.value) return false;
  return intent.value !== 'delegate' || recipient.value !== null;
});
const submitLabel = computed(() => {
  if (intent.value === 'delegate')
    return t(confirming.value ? 'today.command.confirm' : 'today.command.submit.delegate');
  return t(`today.command.submit.${intent.value}`);
});
const hint = computed(() => {
  if (intent.value !== 'delegate') return t(`today.command.hint.${intent.value}`);
  if (props.recipientsStatus === 'error') return t('today.command.recipientsFailed');
  if (props.recipientsStatus === 'ok' && props.recipients.length === 0) return t('today.command.noRecipients');
  if (!recipient.value) return t('today.command.pickRecipient');
  return t(confirming.value ? 'today.command.confirmHint' : 'today.command.hint.delegate', {
    name: recipient.value.name,
  });
});

// 改了内容、意图或对象，之前的确认就不作数了。
watch([text, intent, recipient], () => {
  confirming.value = false;
});
watch(draft, (value) => {
  error.value = '';
  errorLink.value = '';
  if (!value.trim()) {
    manualIntent.value = null;
    manualRecipient.value = null;
  }
});

async function onSubmit() {
  if (!canSubmit.value) return;
  let request: CommandRequest;
  if (intent.value === 'delegate') {
    if (!recipient.value) return;
    if (!confirming.value) {
      confirming.value = true;
      return;
    }
    request = { intent: 'delegate', text: text.value, recipient: recipient.value };
  } else {
    request = { intent: intent.value, text: text.value };
  }
  busy.value = true;
  const failure = await props.run(request);
  busy.value = false;
  // 提交期间输入框被禁用会丢焦点：办完把焦点还回来，方便接着下一条。
  void nextTick(() => inputEl.value?.focus());
  if (failure) {
    error.value = failure.message;
    errorLink.value = failure.link ?? '';
    confirming.value = false;
    return;
  }
  draft.value = '';
}

function onEscape() {
  if (confirming.value) confirming.value = false;
  else draft.value = '';
}

defineExpose({ focus: () => inputEl.value?.focus() });
</script>
