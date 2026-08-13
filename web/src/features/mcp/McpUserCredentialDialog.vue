// Container: approved — feature-local dialog; credential CRUD lives in useMcpUserCredentialDialog composable.
<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
      <q-card-section class="row items-center justify-between">
        <div>
          <div class="text-h6">用户凭据</div>
          <div class="text-caption text-grey-7">{{ serverLabel }} · {{ userLabel || `用户 ${userId}` }}</div>
        </div>
        <q-btn flat dense round icon="close" aria-label="关闭" @click="$emit('update:modelValue', false)" />
      </q-card-section>
      <q-separator />
      <q-card-section class="app-dialog-body">
        <q-inner-loading :showing="loading" />
        <div v-if="!loading && items.length === 0" class="text-caption text-grey-7 q-mb-md">
          尚未配置凭据。保存后 Agent 会话方可调用此 MCP。
        </div>
        <q-list v-if="items.length" bordered separator class="rounded-borders q-mb-md">
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
          <q-input
            v-model="form.credential_key"
            dense
            outlined
            label="凭据键"
            hint="通常为 Authorization 或 API 头名"
          />
          <q-input v-model="form.secret" dense outlined type="password" label="密钥 / Token" />
          <div class="app-actions-bar app-actions-bar--start">
            <q-btn
              color="primary"
              unelevated
              rounded
              no-caps
              label="保存凭据"
              type="submit"
              :loading="saving"
              :disable="!canSave"
            />
          </div>
        </q-form>
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { toRef } from 'vue';
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
