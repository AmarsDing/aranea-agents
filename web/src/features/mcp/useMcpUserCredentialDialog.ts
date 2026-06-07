// FB3+FB4 fix: extract credential CRUD + $q.notify from McpUserCredentialDialog.vue
// into composable so the .vue file only handles template bindings.
import { computed, reactive, ref, watch, type Ref } from 'vue';
import { useQuasar } from 'quasar';
import type { McpUserCredential } from '../types';
import { useMcpStore } from '../../stores/mcp';

export function useMcpUserCredentialDialog(
  modelValue: Ref<boolean>,
  mcpServerId: Ref<string>,
  userId: Ref<string>,
  emit: { (e: 'saved'): void },
) {
  const $q = useQuasar();
  const mcpStore = useMcpStore();
  const loading = ref(false);
  const saving = ref(false);
  const items = ref<McpUserCredential[]>([]);
  const form = reactive({ credential_key: 'Authorization', secret: '' });

  const canSave = computed(
    () => Boolean(mcpServerId.value && userId.value && form.credential_key.trim() && form.secret.trim()),
  );

  watch(
    [modelValue, mcpServerId, userId] as const,
    ([open, serverId, uid]) => {
      if (open && serverId && uid) void reload();
    },
  );

  async function reload() {
    if (!mcpServerId.value || !userId.value) return;
    loading.value = true;
    try {
      items.value = await mcpStore.fetchUserCredentials(mcpServerId.value, userId.value);
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '加载凭据失败' });
    } finally {
      loading.value = false;
    }
  }

  async function save() {
    if (!canSave.value) return;
    saving.value = true;
    try {
      await mcpStore.saveUserCredential(mcpServerId.value, userId.value, {
        credential_key: form.credential_key.trim(),
        secret: form.secret.trim(),
      });
      form.secret = '';
      await reload();
      emit('saved');
      $q.notify({ type: 'positive', message: '凭据已保存' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '保存失败' });
    } finally {
      saving.value = false;
    }
  }

  async function remove(credentialKey: string) {
    try {
      await mcpStore.removeUserCredential(mcpServerId.value, userId.value, credentialKey);
      await reload();
      $q.notify({ type: 'positive', message: '已删除' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '删除失败' });
    }
  }

  function confirmRemove(credentialKey: string) {
    $q.dialog({
      title: '删除凭据',
      message: `确定删除凭据「${credentialKey}」？删除后 Agent 将无法使用该凭据访问 MCP 服务。`,
      cancel: true,
      persistent: true,
    }).onOk(() => void remove(credentialKey));
  }

  return { loading, saving, items, form, canSave, reload, save, confirmRemove };
}
