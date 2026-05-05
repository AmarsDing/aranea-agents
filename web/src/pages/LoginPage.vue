<template>
  <q-layout view="hHh Lpr lff">
    <q-page-container>
      <q-page class="row items-center justify-center login-page">
        <q-card flat bordered class="login-card" :class="{ 'login-card--dark': isDark }">
          <q-card-section class="text-center">
            <div class="text-h5 text-weight-bold">{{ t("common.appTitle") }}</div>
            <div class="text-caption text-grey-7 q-mt-xs">{{ t("auth.subtitle") }}</div>
          </q-card-section>
          <q-separator />
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

          <q-card-actions vertical class="q-px-md q-pb-lg">
            <q-btn color="primary" unelevated :loading="auth.loginLoading" :label="t('auth.submit')" padding="sm md" @click="submit" />
            <div class="text-caption text-grey-7 text-center q-mt-sm">{{ t("auth.backendHint") }}</div>
          </q-card-actions>
        </q-card>
      </q-page>
    </q-page-container>
  </q-layout>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useQuasar } from "quasar";
import { useI18n } from "vue-i18n";
import { useAuthStore, type AuthIdentityMode } from "../stores/auth";

const { t } = useI18n();
const $q = useQuasar();
const route = useRoute();
const router = useRouter();
const auth = useAuthStore();

const isDark = computed(() => $q.dark.isActive);
const mode = ref<AuthIdentityMode>("username");
const identity = ref("");
const password = ref("");
const showPwd = ref(false);
const localError = ref("");

watch(mode, () => {
  localError.value = "";
});

async function bootstrapIfAlreadyAuthed() {
  await auth.ensureSession();
  if (auth.user) {
    await router.replace(typeof route.query.redirect === "string" ? route.query.redirect : "/overview");
  }
}

void bootstrapIfAlreadyAuthed();

async function submit() {
  localError.value = "";
  try {
    await auth.login(mode.value, identity.value, password.value);
    password.value = "";
    const redirect = typeof route.query.redirect === "string" && route.query.redirect.startsWith("/") ? route.query.redirect : "/overview";
    await router.replace(redirect || "/overview");
  } catch {
    localError.value = t("auth.loginFailed");
    $q.notify({ type: "negative", message: t("auth.loginFailed") });
  }
}
</script>

<style scoped>
.login-page {
  padding: 24px;
}
.login-card {
  width: 100%;
  max-width: 420px;
  border-radius: 20px !important;
  box-shadow: 0 22px 50px rgba(15, 23, 42, 0.08);
}
.login-card--dark {
  box-shadow: 0 22px 50px rgba(0, 0, 0, 0.35);
  border-color: rgba(255, 255, 255, 0.08) !important;
}
</style>
