// Container: approved — platform memory policy toggles (persisted to system_settings).
<template>
  <q-card flat bordered class="memory-card">
    <q-card-section class="row items-center justify-between">
      <div>
        <div class="text-h6">平台记忆策略</div>
        <div class="text-caption text-grey-7">
          持久化到 system_settings；进程环境变量（MEMORY_POLICY_STRICT / MEMORY_EPISODE_BACKFILL_DISABLED）优先于 UI
          值。
        </div>
      </div>
      <q-btn flat dense icon="refresh" :loading="loading" @click="load" />
    </q-card-section>

    <q-card-section v-if="loaded" class="column q-gutter-md">
      <q-toggle
        v-model="form.policy_strict"
        color="warning"
        label="Policy Strict（审计失败阻断写路径）"
        :disable="envPolicyStrict"
      />
      <div v-if="envPolicyStrict" class="text-caption text-orange-8">
        已由环境变量 MEMORY_POLICY_STRICT=1 强制启用，UI 不可关闭。
      </div>

      <q-toggle
        v-model="form.episode_backfill_disabled"
        color="primary"
        label="禁用 Episode Embedding Backfill"
        :disable="envBackfillDisabled"
      />
      <div v-if="envBackfillDisabled" class="text-caption text-orange-8">
        已由环境变量 MEMORY_EPISODE_BACKFILL_DISABLED=1 强制禁用，UI 不可开启。
      </div>

      <div class="row q-gutter-sm">
        <q-btn color="primary" label="保存" :loading="saving" @click="save" />
      </div>

      <q-banner v-if="message" rounded :class="messageOk ? 'bg-positive text-white' : 'bg-negative text-white'">
        {{ message }}
      </q-banner>
    </q-card-section>

    <q-card-section v-else-if="!loading" class="text-grey-7 text-caption">平台设置加载失败。</q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useMemoryApi } from './composables/useMemoryApi';
const { getMemoryPlatformSettings, updateMemoryPlatformSettings } = useMemoryApi();

const loading = ref(false);
const saving = ref(false);
const loaded = ref(false);
const envPolicyStrict = ref(false);
const envBackfillDisabled = ref(false);
const message = ref('');
const messageOk = ref(true);

const form = reactive({
  policy_strict: false,
  episode_backfill_disabled: false,
});

async function load() {
  loading.value = true;
  message.value = '';
  try {
    const row = await getMemoryPlatformSettings();
    form.policy_strict = row.policy_strict;
    form.episode_backfill_disabled = row.episode_backfill_disabled;
    envPolicyStrict.value = row.env_policy_strict_override;
    envBackfillDisabled.value = row.env_episode_backfill_disabled_override;
    loaded.value = true;
  } catch (err) {
    loaded.value = false;
    message.value = err instanceof Error ? err.message : '加载失败';
    messageOk.value = false;
  } finally {
    loading.value = false;
  }
}

async function save() {
  saving.value = true;
  message.value = '';
  try {
    const row = await updateMemoryPlatformSettings({
      policy_strict: form.policy_strict,
      episode_backfill_disabled: form.episode_backfill_disabled,
    });
    form.policy_strict = row.policy_strict;
    form.episode_backfill_disabled = row.episode_backfill_disabled;
    envPolicyStrict.value = row.env_policy_strict_override;
    envBackfillDisabled.value = row.env_episode_backfill_disabled_override;
    message.value = '已保存';
    messageOk.value = true;
  } catch (err) {
    message.value = err instanceof Error ? err.message : '保存失败';
    messageOk.value = false;
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>
