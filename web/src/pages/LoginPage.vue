<template>
  <q-layout view="hHh Lpr lff">
    <q-page-container>
      <q-page class="row items-center justify-center login-page">
        <q-card flat bordered class="login-card">
          <q-card-section class="text-center">
            <div class="text-h5 text-weight-bold">{{ t('common.appTitle') }}</div>
            <div class="text-caption text-grey-7 q-mt-xs">{{ t('auth.subtitle') }}</div>
          </q-card-section>
          <q-separator />

          <template v-if="backendChecking">
            <q-card-section class="text-center q-py-lg">
              <q-spinner-dots color="primary" size="40px" />
              <div class="text-caption text-grey-7 q-mt-sm">{{ t('auth.checkingBackend') }}</div>
            </q-card-section>
          </template>

          <template v-else-if="backendStarting">
            <q-card-section class="text-center q-py-lg">
              <q-spinner-dots color="primary" size="40px" />
              <div class="text-caption text-grey-7 q-mt-sm">{{ t('auth.backendStarting') }}</div>
            </q-card-section>
          </template>

          <template v-else-if="!backendHealthy">
            <q-card-section>
              <q-banner rounded dense class="bg-negative text-white">
                <template #avatar>
                  <q-icon name="cloud_off" />
                </template>
                {{ t('auth.backendDown') }}
              </q-banner>
              <div class="text-center q-mt-md">
                <q-btn
                  outline
                  color="primary"
                  :loading="rechecking"
                  :label="t('auth.recheck')"
                  @click="recheckBackend"
                />
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
                  <q-btn
                    flat
                    round
                    dense
                    :icon="showPwd ? 'visibility_off' : 'visibility'"
                    tabindex="-1"
                    @click.stop="showPwd = !showPwd"
                  />
                </template>
              </q-input>

              <q-banner v-if="localError" rounded dense class="bg-negative text-white q-mt-md">
                {{ localError }}
              </q-banner>
            </q-card-section>

            <q-banner v-if="authBypass" rounded dense class="bg-info text-white q-mx-md q-mb-sm">
              {{ t('auth.devBypassHint') }}
            </q-banner>

            <q-card-actions vertical class="q-px-md q-pb-lg">
              <q-btn
                color="primary"
                unelevated
                :loading="auth.loginLoading"
                :label="t('auth.submit')"
                padding="sm md"
                @click="submit"
              />
              <q-btn
                v-if="authBypass"
                flat
                color="primary"
                class="q-mt-xs"
                :label="t('auth.enterWithoutLogin')"
                @click="enterWithoutLogin"
              />
              <div v-if="!authBypass" class="text-caption text-grey-7 text-center q-mt-sm">
                {{ t('auth.defaultAccountHint') }}
              </div>
              <div class="text-caption text-grey-7 text-center q-mt-sm">{{ t('auth.backendHint') }}</div>
            </q-card-actions>
          </template>
        </q-card>
      </q-page>
    </q-page-container>
  </q-layout>
</template>

<script setup lang="ts">
import { useLoginPage } from '../features/admin/useLoginPage';

const {
  t,
  auth,
  mode,
  identity,
  password,
  showPwd,
  localError,
  backendChecking,
  backendHealthy,
  backendStarting,
  rechecking,
  authBypass,
  recheckBackend,
  bootstrapIfAlreadyAuthed,
  enterWithoutLogin,
  submit,
} = useLoginPage();

void bootstrapIfAlreadyAuthed();
</script>
