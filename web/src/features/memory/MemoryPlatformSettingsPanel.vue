// Container: approved — platform memory policy toggles (persisted to system_settings). // FD4+FB3 fix: data
fetching/saving + error handling extracted to useMemoryPlatformSettings composable.
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
import { useMemoryPlatformSettings } from './composables/useMemoryPlatformSettings';

const { loading, saving, loaded, envPolicyStrict, envBackfillDisabled, message, messageOk, form, load, save } =
  useMemoryPlatformSettings();
</script>
