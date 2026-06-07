// TECH-DEBT(FL5): composable 直接调用 heartbeat/api 而非通过 Store，
// 因为登录页的健康检查是一次性操作且不需要全局状态。
import { ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { useAuthStore, type AuthIdentityMode } from '../../stores/auth';
import { fetchAuthHealth } from '../heartbeat/api';
import { formatLoginError } from './loginErrors';
import { checkBackendHealth, getServerHeartbeatState } from '../heartbeat/useServerHeartbeat';

export function useLoginPage() {
  const { t } = useI18n();
  const $q = useQuasar();
  const route = useRoute();
  const router = useRouter();
  const auth = useAuthStore();

  const mode = ref<AuthIdentityMode>('username');
  const identity = ref('');
  const password = ref('');
  const showPwd = ref(false);
  const localError = ref('');

  const backendChecking = ref(true);
  const backendHealthy = ref(false);
  const rechecking = ref(false);
  const authBypass = ref(false);

  watch(mode, () => {
    localError.value = '';
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
    if (health?.auth_mode === 'bypass') {
      authBypass.value = true;
    }
    const healthy = health?.status === 'ok' || (await checkBackendHealth());
    backendHealthy.value = healthy;
    backendChecking.value = false;
  }

  async function recheckBackend() {
    rechecking.value = true;
    const health = await fetchAuthHealth();
    authBypass.value = health?.auth_mode === 'bypass';
    const healthy = health?.status === 'ok' || (await checkBackendHealth());
    backendHealthy.value = healthy;
    rechecking.value = false;
  }

  async function bootstrapIfAlreadyAuthed() {
    await checkBackend();
    if (!backendHealthy.value) return;
    await auth.ensureSession();
    if (auth.user) {
      await router.replace(typeof route.query.redirect === 'string' ? route.query.redirect : '/overview');
    }
  }

  async function enterWithoutLogin() {
    const redirect =
      typeof route.query.redirect === 'string' && route.query.redirect.startsWith('/')
        ? route.query.redirect
        : '/overview';
    await router.replace(redirect || '/overview');
  }

  async function submit() {
    localError.value = '';
    try {
      await auth.login(mode.value, identity.value, password.value);
      password.value = '';
      const redirect =
        typeof route.query.redirect === 'string' && route.query.redirect.startsWith('/')
          ? route.query.redirect
          : '/overview';
      await router.replace(redirect || '/overview');
    } catch (err) {
      const info = formatLoginError(err, t);
      localError.value = info.message;
      $q.notify({ type: 'negative', message: info.message });
    }
  }

  return {
    t,
    auth,
    mode,
    identity,
    password,
    showPwd,
    localError,
    backendChecking,
    backendHealthy,
    rechecking,
    authBypass,
    checkBackend,
    recheckBackend,
    bootstrapIfAlreadyAuthed,
    enterWithoutLogin,
    submit,
  };
}
