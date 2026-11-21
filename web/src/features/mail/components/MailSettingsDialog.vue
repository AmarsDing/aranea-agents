<template>
  <!-- 邮件设置：每个账户的签名、发出的信是否存到服务器「已发送」；拒收名单 -->
  <AppDialog
    :model-value="modelValue"
    size="md"
    position="standard"
    :title="t('mailPage.settings.title')"
    :loading="busy"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="ml-set">
      <template v-if="accounts.length">
        <q-select
          v-model="accountId"
          dense
          outlined
          emit-value
          map-options
          :options="accounts.map((a) => ({ value: a.id, label: a.name || a.email || a.username }))"
          :label="t('mailPage.settings.account')"
        />
        <q-input
          v-model="signature"
          outlined
          dense
          type="textarea"
          autogrow
          maxlength="2000"
          :label="t('mailPage.settings.signature')"
          :hint="t('mailPage.settings.signatureHint')"
        />
        <q-toggle v-model="saveSent" :label="t('mailPage.settings.saveSent')" />
        <p class="ml-hint">{{ t('mailPage.settings.saveSentHint') }}</p>
        <div>
          <q-btn
            unelevated
            no-caps
            color="primary"
            :label="t('mailPage.settings.save')"
            :loading="busy"
            :disable="!dirty"
            @click="saveAccount"
          />
        </div>
      </template>

      <div class="ml-sect">{{ t('mailPage.settings.blocked') }}</div>
      <p class="ml-hint">{{ t('mailPage.settings.blockedHint') }}</p>
      <div class="ml-set__row">
        <q-input
          v-model="newBlock"
          dense
          outlined
          class="ml-set__grow"
          :placeholder="t('mailPage.settings.blockPlaceholder')"
          @keydown.enter.prevent="addBlock"
        />
        <q-btn
          flat
          no-caps
          icon="block"
          :label="t('mailPage.settings.block')"
          :disable="!newBlock.trim()"
          @click="addBlock"
        />
      </div>
      <p v-if="!blocked.length" class="ml-hint">{{ t('mailPage.settings.blockedNone') }}</p>
      <div v-for="a in blocked" :key="a" class="ml-set__row">
        <span class="ml-num ml-set__grow">{{ a }}</span>
        <q-btn
          flat
          dense
          round
          size="sm"
          icon="close"
          :aria-label="t('mailPage.settings.unblock', { a })"
          @click="removeBlock(a)"
        >
          <q-tooltip>{{ t('mailPage.settings.unblock', { a }) }}</q-tooltip>
        </q-btn>
      </div>
    </div>
  </AppDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import AppDialog from '../../../components/common/AppDialog.vue';
import { blockSender, listBlockedSenders, unblockSender, updateMailAccountSettings } from '../../office/api';
import type { MailAccountRow } from '../../office/types';

const props = defineProps<{ modelValue: boolean; accounts: MailAccountRow[]; initialAccountId?: string }>();
const emit = defineEmits<{ 'update:modelValue': [value: boolean]; saved: [] }>();

const { t } = useI18n();
const $q = useQuasar();

const accountId = ref('');
const signature = ref('');
const saveSent = ref(false);
const blocked = ref<string[]>([]);
const newBlock = ref('');
const busy = ref(false);

const current = computed(() => props.accounts.find((a) => a.id === accountId.value));
const dirty = computed(
  () => !!current.value && (signature.value !== current.value.signature || saveSent.value !== current.value.save_sent),
);

function fail(e: unknown) {
  $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
}

watch(
  () => props.modelValue,
  async (open) => {
    if (!open) return;
    accountId.value = props.initialAccountId || props.accounts[0]?.id || '';
    try {
      blocked.value = await listBlockedSenders();
    } catch (e) {
      fail(e);
    }
  },
);
watch(current, (a) => {
  signature.value = a?.signature ?? '';
  saveSent.value = a?.save_sent ?? false;
});

async function saveAccount() {
  if (!current.value) return;
  busy.value = true;
  try {
    await updateMailAccountSettings(current.value.id, { signature: signature.value, save_sent: saveSent.value });
    $q.notify({ type: 'positive', message: t('mailPage.settings.saved') });
    emit('saved');
  } catch (e) {
    fail(e);
  } finally {
    busy.value = false;
  }
}

async function addBlock() {
  const a = newBlock.value.trim();
  if (!a) return;
  try {
    const n = await blockSender(a);
    newBlock.value = '';
    blocked.value = await listBlockedSenders();
    $q.notify({ message: t('mailPage.settings.blockedDone', { n }) });
    emit('saved');
  } catch (e) {
    fail(e);
  }
}

async function removeBlock(a: string) {
  try {
    await unblockSender(a);
    blocked.value = blocked.value.filter((x) => x !== a);
  } catch (e) {
    fail(e);
  }
}
</script>
