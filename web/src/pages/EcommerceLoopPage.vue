<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { SCENES, SHOPS, STEPS } from '../features/ecommerce-loop/data';
import { useEcommerceLoopPage } from '../features/ecommerce-loop/useEcommerceLoopPage';
import CompanyScene from '../components/ecommerce-loop/CompanyScene.vue';
import FlowScene from '../components/ecommerce-loop/FlowScene.vue';
import StageHeader from '../components/ecommerce-loop/StageHeader.vue';
import TrafficScene from '../components/ecommerce-loop/TrafficScene.vue';
import SummaryScene from '../components/ecommerce-loop/SummaryScene.vue';
import WorkflowDrawer from '../components/ecommerce-loop/WorkflowDrawer.vue';
import CommerceWorkbench from '../components/ecommerce-loop/CommerceWorkbench.vue';
const { t } = useI18n();
const {
  root,
  viewport,
  scale,
  viewMode,
  activeCase,
  isFullscreen,
  scene,
  focusStep,
  referenceFocus,
  referenceOptions,
  companyFocused,
  selectedShop,
  selectedProduct,
  currentCases,
  storageWarning,
  cameraStyle,
  drawerOpen,
  editing,
  selectedWorkflow,
  shopName,
  onNavigate,
  onOverview,
  onOpenWorkflow,
  onCloseDrawer,
  onWorkflowAction,
  onTrafficDecision,
  onReset,
  onFullscreen,
  onShopChange,
  onViewChange,
  onCaseChange,
  onDirty,
  onDrawerHidden,
  onEnterShop,
  onFocusTask,
  setScene,
  focus,
  onReferenceFocus,
  selectShop,
  onFocusCompany,
  onSelectProduct,
} = useEcommerceLoopPage();
</script>

<template>
  <q-page class="ec-page">
    <div ref="root" class="ec-experience" :class="{ 'ec-experience--workbench': viewMode === 'workbench' }">
      <header class="ec-toolbar">
        <div class="ec-toolbar-title">
          <q-icon name="shopping_bag" /><b>{{
            t(viewMode === 'workbench' ? 'ecommerceLoop.workbench.title' : 'ecommerceLoop.title')
          }}</b
          ><span>{{ t(viewMode === 'workbench' ? 'ecommerceLoop.workbench.subtitle' : 'ecommerceLoop.mode') }}</span>
        </div>
        <nav class="ec-view-switch" :aria-label="t('ecommerceLoop.workbench.views')">
          <button :aria-pressed="viewMode === 'workbench'" @click="onViewChange('workbench')">
            {{ t('ecommerceLoop.workbench.view') }}
          </button>
          <button :aria-pressed="viewMode === 'reference'" @click="onViewChange('reference')">
            {{ t('ecommerceLoop.workbench.referenceView') }}
          </button>
        </nav>
        <nav v-if="viewMode === 'reference'" class="ec-scene-tabs" :aria-label="t('ecommerceLoop.title')">
          <button
            v-for="id in SCENES"
            :key="id"
            :aria-current="scene === id ? 'page' : undefined"
            :class="{ 'is-active': scene === id }"
            @click="setScene(id)"
          >
            <small>0{{ id }}</small
            >{{ t(`ecommerceLoop.tabs.${id}`) }}
          </button>
        </nav>
        <div class="ec-toolbar-actions">
          <q-btn flat dense round icon="restart_alt" :aria-label="t('ecommerceLoop.reset')" @click="onReset" /><q-btn
            flat
            dense
            round
            :icon="isFullscreen ? 'fullscreen_exit' : 'fullscreen'"
            :aria-label="t(isFullscreen ? 'ecommerceLoop.exitFullscreen' : 'ecommerceLoop.fullscreen')"
            @click="onFullscreen"
          />
        </div>
      </header>
      <CommerceWorkbench
        v-if="viewMode === 'workbench'"
        :selected-shop="selectedShop"
        :selected-product="selectedProduct"
        :active-case="activeCase"
        :cases="currentCases"
        :editing="editing"
        :storage-warning="storageWarning"
        @select-shop="selectShop"
        @select-product="onSelectProduct"
        @select-case="onCaseChange"
        @action="onWorkflowAction"
        @dirty="onDirty"
        @focus-task="onFocusTask"
      />
      <nav
        v-if="viewMode === 'reference' && (referenceOptions.length || companyFocused)"
        class="ec-step-navigation"
        :aria-label="t('ecommerceLoop.workbench.steps')"
      >
        <button
          v-for="step in scene === 2 || scene === 4 ? STEPS : []"
          :key="step"
          :aria-pressed="focusStep === step"
          @click="focus(step)"
        >
          {{ step }} · {{ t(`ecommerceLoop.steps.${step}.title`) }}
        </button>
        <button
          v-for="option in referenceOptions"
          :key="option"
          :data-reference-focus="option"
          :aria-pressed="referenceFocus === option"
          @click="onReferenceFocus(option)"
        >
          {{ t(`ecommerceLoop.referenceFocus.${option}`) }}
        </button>
        <button data-overview-control @click="onOverview">{{ t('ecommerceLoop.overview') }}</button>
        <span class="ec-reference-caption">{{ t('ecommerceLoop.referenceHint') }}</span>
      </nav>
      <div v-if="viewMode === 'reference'" ref="viewport" class="ec-viewport">
        <div class="ec-stage-fit" :style="{ width: `${1920 * scale}px`, height: `${1080 * scale}px` }">
          <div class="ec-stage" :style="{ transform: `scale(${scale})` }">
            <div class="ec-camera" :style="cameraStyle">
              <StageHeader :scene="scene" />
              <CompanyScene
                v-if="scene === 1"
                :focused="companyFocused"
                :selected-shop="selectedShop"
                @focus-commerce="onFocusCompany"
                @open-loop="setScene(2)"
                @select-shop="onEnterShop"
              />
              <FlowScene
                v-else-if="scene === 2 || scene === 4"
                :key="scene"
                :launch="scene === 4"
                :focus-step="focusStep"
                :reference-focus="referenceFocus"
                :workflow="scene === 4 ? currentCases.launch : currentCases.traffic"
                @focus="focus"
                @reference-focus="onReferenceFocus"
              />
              <TrafficScene
                v-else-if="scene === 3"
                :workflow="currentCases.traffic"
                :selected-product="selectedProduct"
                :reference-focus="referenceFocus"
                @decide="onTrafficDecision"
                @select-product="onSelectProduct"
                @operate="onOpenWorkflow('traffic')"
              />
              <SummaryScene v-else />
            </div>
            <div v-if="scene === 2 || scene === 4" class="ec-focus-markers">
              <button
                v-for="step in STEPS"
                :key="step"
                :class="{ 'is-active': focusStep === step }"
                :aria-label="t('ecommerceLoop.focus', { step })"
                :aria-pressed="focusStep === step"
                tabindex="-1"
                @click="focus(step)"
              />
            </div>
          </div>
        </div>
      </div>
      <footer v-if="viewMode === 'reference'" class="ec-controls">
        <div class="ec-controls-caption">
          <span class="ec-demo-dot" />{{ t('ecommerceLoop.local')
          }}<small v-if="storageWarning">{{ t(`ecommerceLoop.storage.${storageWarning}`) }}</small
          ><small v-else>{{ t('ecommerceLoop.demoNotice') }}</small>
        </div>
        <div class="ec-controls-center">
          <q-btn
            flat
            dense
            round
            icon="chevron_left"
            :disable="scene === 1"
            :aria-label="t('ecommerceLoop.previous')"
            @click="onNavigate(-1)"
          /><span>0{{ scene }} / 05</span
          ><q-btn
            flat
            dense
            round
            icon="chevron_right"
            :disable="scene === 5"
            :aria-label="t('ecommerceLoop.next')"
            @click="onNavigate(1)"
          />
        </div>
        <div class="ec-controls-actions">
          <select :value="selectedShop" :aria-label="t('ecommerceLoop.selectShop')" @change="onShopChange">
            <option v-for="shop in SHOPS" :key="shop" :value="shop">
              {{ t('ecommerceLoop.shop', { name: shop.toUpperCase() }) }}
            </option></select
          ><q-btn
            flat
            no-caps
            icon="play_circle_outline"
            :label="
              t('ecommerceLoop.workbench.openTask', {
                name: t(`ecommerceLoop.workflow.${scene === 4 ? 'launch' : 'traffic'}`),
              })
            "
            @click="onOpenWorkflow()"
          />
        </div>
      </footer>
    </div>
    <WorkflowDrawer
      v-if="viewMode === 'reference'"
      :open="drawerOpen"
      :workflow="selectedWorkflow"
      :editing="editing"
      :shop-name="shopName"
      @close="onCloseDrawer"
      @action="onWorkflowAction"
      @dirty="onDirty"
      @hidden="onDrawerHidden"
    />
  </q-page>
</template>
