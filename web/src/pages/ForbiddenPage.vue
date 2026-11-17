<template>
  <q-page class="app-standard-page">
    <AppStatePanel
      variant="forbidden"
      :title="t('shell.forbidden.title')"
      :description="description"
      icon="lock"
      :action-label="t('shell.forbidden.backHome')"
      action-icon="dashboard"
      :secondary-label="t('shell.forbidden.goBack')"
      @action="router.replace('/workbench')"
      @secondary="goBack"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRoute, useRouter } from 'vue-router';
import AppStatePanel from '../components/common/AppStatePanel.vue';

const { t } = useI18n();
const route = useRoute();
const router = useRouter();

const fromPath = computed(() => {
  const from = route.query.from;
  return typeof from === 'string' && from ? from : '';
});

const description = computed(() => {
  const parts: string[] = [];
  if (fromPath.value) parts.push(t('shell.forbidden.desc', { path: fromPath.value }));
  else parts.push(t('shell.state.forbiddenDesc'));
  const need = route.query.need;
  if (typeof need === 'string' && need) parts.push(t('shell.forbidden.needPermission', { permission: need }));
  return parts.join(' ');
});

function goBack() {
  if (window.history.length > 1) router.back();
  else router.replace('/workbench');
}
</script>
