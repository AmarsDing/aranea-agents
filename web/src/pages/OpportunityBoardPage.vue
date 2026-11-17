<template>
  <q-page class="app-standard-page app-registry-page">
    <AppPageHero
      :title="t('uiSystem.operations.opportunityTitle')"
      :subtitle="t('uiSystem.operations.opportunitySubtitle')"
    />
    <!-- 五态：已有机会但刷新失败 → 局部失败横幅，列表照常渲染 -->
    <AppStatePanel
      v-if="(error || forbidden) && rows.length > 0"
      variant="partial"
      dense
      class="q-mb-md"
      :description="error"
      :action-label="t('uiSystem.operations.retry')"
      action-icon="refresh"
      @action="refresh"
    />
    <!-- 五态：无权限 → 只渲染 forbidden 面板，不提供重试 -->
    <AppStatePanel v-else-if="forbidden" variant="forbidden" class="q-mb-md" />
    <!-- 五态：加载失败且无机会 → 只渲染可重试错误面板，不与空态并存。 -->
    <AppStatePanel
      v-else-if="error"
      variant="error"
      class="q-mb-md"
      :description="error"
      :action-label="t('uiSystem.operations.retry')"
      action-icon="refresh"
      @action="refresh"
    />
    <q-card flat class="app-registry-panel"
      ><q-card-section>
        <div class="ui-preview-actions q-mb-md">
          <q-btn-toggle
            v-model="audience"
            toggle-color="primary"
            :options="ui.audienceOptions"
            :disable="saving"
            @update:model-value="refresh"
          /><q-btn
            flat
            round
            icon="refresh"
            :aria-label="t('uiSystem.operations.refreshOpportunities')"
            :loading="loading"
            @click="refresh"
          />
        </div>
        <q-form class="ui-operations-form" @submit.prevent="create"
          ><q-input
            v-model="title"
            dense
            outlined
            :label="t('uiSystem.operations.opportunityRequired')"
            :rules="[required]"
            class="app-field-md" /><q-btn
            type="submit"
            color="primary"
            unelevated
            :label="t('uiSystem.operations.createOpportunity')"
            :loading="saving"
            :disable="!title.trim() || saving || loading"
        /></q-form> </q-card-section
    ></q-card>
    <!-- 五态：首屏加载无机会 → 骨架面板 -->
    <AppStatePanel v-if="loading && rows.length === 0 && !error && !forbidden" variant="loading" :rows="4" />
    <!-- 五态：成功但无机会 → 空态 -->
    <AppStatePanel
      v-else-if="!loading && !error && !forbidden && rows.length === 0"
      variant="empty"
      :title="t('uiSystem.operations.noOpportunities')"
      icon="lightbulb"
    />
    <q-card v-else-if="rows.length > 0" flat
      ><q-list separator :aria-busy="loading">
        <q-item v-for="item in rows" :key="item.ID" class="ui-operations-row"
          ><q-item-section
            ><q-item-label class="app-registry-cell-primary">{{ item.Title }}</q-item-label
            ><q-item-label caption>{{ item.Audience }} · {{ item.Status }}</q-item-label
            ><q-item-label v-if="item.Summary" caption>{{ item.Summary }}</q-item-label></q-item-section
          ><q-item-section side
            ><div class="ui-preview-actions">
              <q-btn
                flat
                color="primary"
                :label="t('uiSystem.operations.confirm')"
                :disable="saving || loading"
                @click="ack(item.ID)"
              /><q-btn
                outline
                color="primary"
                :label="t('uiSystem.operations.continueSourcing')"
                :to="{ name: 'sourcing', query: { opportunity_id: item.ID, title: item.Title } }"
              /></div></q-item-section
        ></q-item>
      </q-list></q-card
    >
  </q-page>
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';
import { useOpportunityBoardStore } from '../stores/mvp';
import { createMvpUi } from '../features/mvp/mvpUi';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
const { t } = useI18n();
const ui = computed(() => createMvpUi(t));
const store = useOpportunityBoardStore();
const { rows, loading, saving, error, forbidden } = storeToRefs(store);
const audience = ref('');
const title = ref('');
const required = (value: string) => !!value.trim() || t('uiSystem.operations.opportunityMissing');
const refresh = () => store.load(audience.value);
async function create() {
  if (!title.value.trim() || saving.value || loading.value) return;
  if (await store.create(title.value.trim(), audience.value)) {
    title.value = '';
    await refresh();
  }
}
async function ack(id: string) {
  if (saving.value || loading.value) return;
  if (await store.acknowledge(id)) await refresh();
}
onMounted(refresh);
</script>
