// Container: approved — feature-local dialog; credential CRUD lives in useMcpUserCredentialDialog composable.
<template>
  <AppDialog
    :model-value="modelValue"
    title="用户凭据"
    :subtitle="`${serverLabel} · ${userLabel || `用户 ${userId}`}`"
    size="sm"
    icon="key"
    persistent
    :loading="loading"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <div class="credential-dialog">
      <q-inner-loading :showing="loading" />
      <div v-if="!loading && items.length === 0" class="text-caption app-text-secondary">
        尚未配置凭据。保存后 Agent 会话方可调用此 MCP。
      </div>
      <q-list v-if="items.length" bordered separator class="rounded-borders">
        <q-item v-for="cred in items" :key="cred.credential_key">
          <q-item-section>
            <q-item-label>{{ cred.credential_key }}</q-item-label>
            <q-item-label caption>
              {{ cred.configured ? cred.masked_preview || '已配置' : '未配置' }}
              <span v-if="cred.status"> · {{ cred.status }}</span>
            </q-item-label>
          </q-item-section>
          <q-item-section side>
            <q-btn
              flat
              dense
              round
              icon="delete"
              color="negative"
              aria-label="删除"
              @click="confirmRemove(cred.credential_key)"
            />
          </q-item-section>
        </q-item>
      </q-list>
      <q-form class="app-form-field-grid app-form-field-grid--2col" @submit.prevent="save">
        <q-input v-model="form.credential_key" dense outlined label="凭据键" hint="通常为 Authorization 或 API 头名" />
        <q-input v-model="form.secret" dense outlined type="password" label="密钥 / Token" />
        <div class="app-actions-bar app-actions-bar--start">
          <q-btn
            color="primary"
            unelevated
            rounded
            no-caps
            data-test="credential-save"
            label="保存凭据"
            type="submit"
            :loading="saving"
            :disable="!canSave"
          />
        </div>
      </q-form>
    </div>
  </AppDialog>
</template>

<script setup lang="ts">
import { toRef } from 'vue';
import AppDialog from '../../components/common/AppDialog.vue';
import { useMcpUserCredentialDialog } from './useMcpUserCredentialDialog';

const props = defineProps<{
  modelValue: boolean;
  mcpServerId: string;
  serverLabel: string;
  userId: string;
  userLabel?: string;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  saved: [];
}>();

const { loading, saving, items, form, canSave, save, confirmRemove } = useMcpUserCredentialDialog(
  toRef(props, 'modelValue'),
  toRef(props, 'mcpServerId'),
  toRef(props, 'userId'),
  emit,
);
</script>
