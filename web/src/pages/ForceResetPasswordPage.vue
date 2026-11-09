<template>
  <q-layout view="hHh Lpr lff">
    <q-page-container>
      <q-page class="row items-center justify-center login-page">
        <q-card flat bordered class="login-card">
          <q-card-section class="text-center">
            <div class="text-h5 text-weight-bold">{{ t('auth.forceResetTitle') }}</div>
            <div class="text-caption text-grey-7 q-mt-sm force-reset-desc">{{ t('auth.forceResetDesc') }}</div>
          </q-card-section>
          <q-separator />

          <q-card-section>
            <q-input
              v-model="oldPassword"
              dense
              outlined
              :type="showOld ? 'text' : 'password'"
              :label="t('auth.oldPasswordLabel')"
              autocomplete="current-password"
              class="q-mb-md"
              @keyup.enter="submit"
            >
              <template #append>
                <q-btn
                  flat
                  round
                  dense
                  :icon="showOld ? 'visibility_off' : 'visibility'"
                  tabindex="-1"
                  @click.stop="showOld = !showOld"
                />
              </template>
            </q-input>

            <q-input
              v-model="newPassword"
              dense
              outlined
              :type="showNew ? 'text' : 'password'"
              :label="t('auth.newPasswordLabel')"
              autocomplete="new-password"
              class="q-mb-md"
              @keyup.enter="submit"
            >
              <template #append>
                <q-btn
                  flat
                  round
                  dense
                  :icon="showNew ? 'visibility_off' : 'visibility'"
                  tabindex="-1"
                  @click.stop="showNew = !showNew"
                />
              </template>
            </q-input>

            <q-input
              v-model="confirmPassword"
              dense
              outlined
              :type="showNew ? 'text' : 'password'"
              :label="t('auth.confirmPasswordLabel')"
              autocomplete="new-password"
              @keyup.enter="submit"
            />

            <q-banner v-if="localError" rounded dense class="bg-negative text-white q-mt-md" role="alert">
              {{ localError }}
            </q-banner>
          </q-card-section>

          <q-card-actions vertical class="q-px-md q-pb-lg">
            <q-btn
              color="primary"
              unelevated
              :loading="submitting"
              :label="t('auth.submitChange')"
              padding="sm md"
              @click="submit"
            />
            <q-btn flat color="primary" class="q-mt-xs" :label="t('auth.useAnotherAccount')" @click="switchAccount" />
          </q-card-actions>
        </q-card>
      </q-page>
    </q-page-container>
  </q-layout>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import { changeAdminPassword } from '../features/admin/api';
import { useAuthStore } from '../stores/auth';

// P1 补漏·强制改密门:force_reset 账号登录后唯一可达页(路由守卫钉住)。
// 改密成功即被服务端吊销全部会话,本地清理后回登录页用新密码重登。
const { t } = useI18n();
const router = useRouter();
const auth = useAuthStore();

const oldPassword = ref('');
const newPassword = ref('');
const confirmPassword = ref('');
const showOld = ref(false);
const showNew = ref(false);
const submitting = ref(false);
const localError = ref('');

async function submit() {
  localError.value = '';
  if (newPassword.value.length < 8) {
    localError.value = t('auth.passwordTooShort');
    return;
  }
  if (newPassword.value === oldPassword.value) {
    localError.value = t('auth.passwordSameAsOld');
    return;
  }
  if (newPassword.value !== confirmPassword.value) {
    localError.value = t('auth.passwordMismatch');
    return;
  }
  submitting.value = true;
  try {
    await changeAdminPassword(oldPassword.value, newPassword.value);
  } catch {
    localError.value = t('auth.changeFailed');
    submitting.value = false;
    return;
  }
  // 成功:服务端已吊销全部会话(含本会话),本地清理后回登录页。
  await auth.logout();
  await router.push({ path: '/login' });
}

async function switchAccount() {
  await auth.logout();
  await router.push({ path: '/login' });
}
</script>

<style scoped>
.force-reset-desc {
  max-width: 34em;
  margin-left: auto;
  margin-right: auto;
}
</style>
