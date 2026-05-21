<template>
  <q-layout view="hHh Lpr lff">
    <q-page-container>
      <q-page class="row items-center justify-center login-page">
        <q-card flat bordered class="login-card">
          <q-card-section class="text-center">
            <div class="text-h5 text-weight-bold">{{ t("common.appTitle") }}</div>
            <div class="text-caption text-grey-7 q-mt-xs">{{ t("auth.subtitle") }}</div>
          </q-card-section>
          <q-separator />

          <template v-if="backendChecking">
            <q-card-section class="text-center q-py-lg">
              <q-spinner-dots color="primary" size="40px" />
              <div class="text-caption text-grey-7 q-mt-sm">正在检测后端服务...</div>
            </q-card-section>
          </template>

          <template v-else-if="!backendHealthy">
            <q-card-section>
              <q-banner rounded dense class="bg-negative text-white">
                <template #avatar>
                  <q-icon name="cloud_off" />
                </template>
                {{ t("auth.backendDown") }}
              </q-banner>
              <div class="text-center q-mt-md">
                <q-btn outline color="primary" :loading="rechecking" label="重新检测" @click="recheckBackend" />
              </div>
            </q-card-section>
          </template>

          <template v-else>
            <q-card-section>
              <q-tabs
                v-model="mode"
                dense
                narrow-indicator
                no-caps
                align="justify"
                active-color="primary"
                indicator-color="primary"
                class="q-mb-md"
              >
                <q-tab name="username" :label="t('auth.tabUsername')" />
                <q-tab name="email" :label="t('auth.tabEmail')" />
              </q-tabs>

              <q-input
                v-model.trim="identity"
                dense
                outlined
                clearable
                class="q-mb-md"
                :label="mode === 'email' ? t('auth.emailLabel') : t('auth.usernameLabel')"
                :type="mode === 'email' ? 'email' : 'text'"
                autocomplete="username"
                @keyup.enter="submit"
              />

              <q-input
                v-model="password"
                dense
                outlined
                :type="showPwd ? 'text' : 'password'"
                :label="t('auth.passwordLabel')"
                autocomplete="current-password"
                @keyup.enter="submit"
              >
                <template #append>
                  <q-btn flat round dense :icon="showPwd ? 'visibility_off' : 'visibility'" tabindex="-1" @click.stop="showPwd = !showPwd" />
                </template>
              </q-input>

              <q-banner v-if="localError" rounded dense class="bg-negative text-white q-mt-md">
                {{ localError }}
              </q-banner>
            </q-card-section>

            <q-banner v-if="authBypass" rounded dense class="bg-info text-white q-mx-md q-mb-sm">
              {{ t("auth.devBypassHint") }}
            </q-banner>

            <q-card-actions vertical class="q-px-md q-pb-lg">
              <q-btn color="primary" unelevated :loading="auth.loginLoading" :label="t('auth.submit')" padding="sm md" @click="submit" />
              <q-btn
                v-if="authBypass"
                flat
                color="primary"
                class="q-mt-xs"
                label="进入系统（免登录）"
                @click="enterWithoutLogin"
              />
              <div class="text-caption text-grey-7 text-center q-mt-sm">{{ t("auth.backendHint") }}</div>
            </q-card-actions>
          </template>
        </q-card>
      </q-page>
    </q-page-container>
  </q-layout>
</template>

<script setup lang="ts">
import { ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useQuasar } from "quasar";
import { useI18n } from "vue-i18n";
import { useAuthStore, type AuthIdentityMode } from "../stores/auth";
import { fetchAuthHealth } from "../config/authHealth";
import { formatLoginError } from "../features/admin/loginErrors";
import { checkBackendHealth, getServerHeartbeatState } from "../features/heartbeat/useServerHeartbeat";

const { t } = useI18n();
const $q = useQuasar();
const route = useRoute();
const router = useRouter();
const auth = useAuthStore();

const mode = ref<AuthIdentityMode>("username");
const identity = ref("");
const password = ref("");
const showPwd = ref(false);
const localError = ref("");

const backendChecking = ref(true);
const backendHealthy = ref(false);
const rechecking = ref(false);
const authBypass = ref(false);

watch(mode, () => {
  localError.value = "";
});

async function checkBackend() {
  backendChecking.value = true;
  const heartbeat = getServerHeartbeatState();
  if (heartbeat.connected.value && heartbeat.isAlive.value) {
    backendHealthy.value = true;
    backendChecking.value = false;
    return;
  }
  const health = await fetchAuthHealth();
  if (health?.auth_mode === "bypass") {
    authBypass.value = true;
  }
  const healthy = health?.status === "ok" || (await checkBackendHealth());
  backendHealthy.value = healthy;
  backendChecking.value = false;
}

async function recheckBackend() {
  rechecking.value = true;
  const health = await fetchAuthHealth();
  authBypass.value = health?.auth_mode === "bypass";
  const healthy = health?.status === "ok" || (await checkBackendHealth());
  backendHealthy.value = healthy;
  rechecking.value = false;
}

async function bootstrapIfAlreadyAuthed() {
  await checkBackend();
  if (!backendHealthy.value) return;
  await auth.ensureSession();
  if (auth.user) {
    await router.replace(typeof route.query.redirect === "string" ? route.query.redirect : "/overview");
  }
}

void bootstrapIfAlreadyAuthed();

async function enterWithoutLogin() {
  const redirect = typeof route.query.redirect === "string" && route.query.redirect.startsWith("/") ? route.query.redirect : "/overview";
  await router.replace(redirect || "/overview");
}

async function submit() {
  localError.value = "";
  try {
    await auth.login(mode.value, identity.value, password.value);
    password.value = "";
    const redirect = typeof route.query.redirect === "string" && route.query.redirect.startsWith("/") ? route.query.redirect : "/overview";
    await router.replace(redirect || "/overview");
  } catch (err) {
    const info = formatLoginError(err, t);
    localError.value = info.message;
    $q.notify({ type: "negative", message: info.message });
  }
}
</script>
