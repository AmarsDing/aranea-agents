<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { FLOW_POSITIONS, LAUNCH_POSITIONS, PROGRESS_KEYS, STEPS } from '../../features/ecommerce-loop/data';
import type { DemoWorkflow, ReferenceFocus, StepId } from '../../features/ecommerce-loop/types';
import FlowEdges from './FlowEdges.vue';
import ReferenceIcon from './ReferenceIcon.vue';
import ReferenceCopy from './ReferenceCopy.vue';
const props = defineProps<{
  launch: boolean;
  focusStep: StepId | null;
  referenceFocus: ReferenceFocus | null;
  workflow: DemoWorkflow;
}>();
const emit = defineEmits<{
  (event: 'focus', step: StepId): void;
  (event: 'reference-focus', focus: ReferenceFocus): void;
}>();
const { t } = useI18n();
const tones = { 1: 'data', 2: 'ai', 3: 'ai', 4: 'owner', 5: 'ai', 6: 'ceo', 7: 'data' };
const workflowProgress = computed(
  () =>
    ({
      awaiting_decision: 1,
      rejected: 1,
      approved: 2,
      drafting: 3,
      awaiting_publish_approval: 4,
      verification_pending: 5,
      rework_required: 5,
      awaiting_feedback: 6,
      closed: 7,
    })[props.workflow.phase],
);
// Camera annotations describe the source-video example, not workflow mutations.
const progress = computed(() => {
  if (props.referenceFocus === 'products') return 2;
  if (props.launch && props.focusStep && props.focusStep >= 4)
    return ({ 4: 2, 5: 5, 6: 6, 7: 7 } as Record<number, number>)[props.focusStep]!;
  return workflowProgress.value;
});
function position(step: StepId) {
  const p = (props.launch ? LAUNCH_POSITIONS : FLOW_POSITIONS)[step];
  return { left: `${p.x}px`, top: `${p.y}px`, width: `${p.w}px`, height: `${p.h}px` };
}
function stepDimmed(step: StepId) {
  if (props.focusStep !== null) return props.focusStep !== step;
  return props.referenceFocus === 'products' || (props.referenceFocus === 'ceo-summary' && step !== 4);
}
const ceoDimmed = computed(() => props.focusStep !== null && ![3, 4, 5, 6].includes(props.focusStep));
const productsDimmed = computed(() => props.focusStep !== null && ![4, 5, 6, 7].includes(props.focusStep));
const activeDuty = computed(() =>
  props.referenceFocus === 'ceo-summary' || props.referenceFocus === 'products' || props.focusStep === 4
    ? 'summary'
    : props.focusStep === 5
      ? 'dispatch'
      : props.focusStep === 6
        ? 'verify'
        : null,
);
</script>

<template>
  <section class="ec-flow-scene" :class="{ 'ec-flow-scene--launch': launch }">
    <div class="ec-scene-title">
      <span v-if="launch" class="ec-outline-tag">{{ t('ecommerceLoop.launch.tag') }}</span>
      <h1>
        {{ t(launch ? 'ecommerceLoop.launch.title' : 'ecommerceLoop.loop.title')
        }}<strong>{{ t(launch ? 'ecommerceLoop.launch.accent' : 'ecommerceLoop.loop.accent') }}</strong>
      </h1>
      <span class="ec-scene-subtitle">{{
        t(launch ? 'ecommerceLoop.launch.subtitle' : 'ecommerceLoop.loop.subtitle')
      }}</span>
    </div>
    <span v-if="launch" class="ec-example-notice">{{ t('ecommerceLoop.launch.notice') }}</span>
    <FlowEdges :launch="launch" />
    <button
      v-for="step in STEPS"
      :key="step"
      class="ec-step ec-panel"
      :class="[
        `ec-tone-${tones[step]}`,
        {
          'ec-dim': stepDimmed(step),
          'ec-step--focused': focusStep === step,
          'ec-running-border': step === 5 && workflow.phase === 'drafting',
        },
      ]"
      :style="position(step)"
      :aria-label="t('ecommerceLoop.focus', { step })"
      :aria-pressed="focusStep === step"
      :tabindex="stepDimmed(step) ? -1 : 0"
      :inert="stepDimmed(step)"
      :data-reference-step="step"
      @click="emit('focus', step)"
    >
      <div class="ec-step-title">
        <span class="ec-step-number">0{{ step }}</span>
        <h2>{{ t(`ecommerceLoop.steps.${step}.title`) }}</h2>
        <small v-if="!launch || step === 6">{{ t(`ecommerceLoop.steps.${step}.role`) }}</small
        ><span v-if="[2, 3, 5].includes(step)" class="ec-ai-badge">AI</span
        ><q-icon v-if="step === 4" class="ec-owner-icon" name="person_outline" />
      </div>
      <div class="ec-step-body" :class="{ 'ec-step-body--gold': step === 3 }">
        <span v-if="launch" class="ec-example-tag">{{ t('ecommerceLoop.loop.example') }}</span>
        <template v-if="step === 3 && !launch"
          ><span class="ec-suggestion-lead">{{ t('ecommerceLoop.loop.suggestionLead') }}</span
          >{{ t('ecommerceLoop.loop.suggestionQuote') }}</template
        >
        <ReferenceCopy
          v-else
          :text="t(`ecommerceLoop.${launch ? 'launch.bodies' : 'steps'}.${step}${launch ? '' : '.body'}`)"
        />
      </div>
      <div v-if="!launch" class="ec-principle">
        <b>{{ t('ecommerceLoop.loop.principle') }}</b
        ><span>{{ t(`ecommerceLoop.steps.${step}.principle`) }}</span>
      </div>
    </button>
    <button
      class="ec-flow-ceo ec-panel ec-tone-ceo"
      :class="{ 'ec-flow-ceo--compact': launch, 'ec-dim': ceoDimmed }"
      :inert="ceoDimmed"
      :aria-label="t('ecommerceLoop.referenceFocus.ceo-summary')"
      @click="emit('reference-focus', launch ? 'products' : 'ceo-summary')"
    >
      <div class="ec-flow-ceo-title">
        <ReferenceIcon />
        <h2>CEO Agent</h2>
        <small v-if="!launch">{{ t('ecommerceLoop.loop.manage') }}</small
        ><template v-else
          ><span
            v-for="key in ['summary', 'dispatch', 'verify']"
            :key="key"
            class="ec-ceo-chip"
            :class="{ 'ec-duty-dim': activeDuty !== null && activeDuty !== key }"
            >{{ t(`ecommerceLoop.loop.${key}`) }}</span
          ></template
        >
      </div>
      <template v-if="!launch"
        ><div
          v-for="key in ['summary', 'dispatch', 'verify']"
          :key="key"
          class="ec-ceo-duty"
          :class="{ 'ec-duty-dim': activeDuty !== null && activeDuty !== key }"
        >
          <b>{{ t(`ecommerceLoop.loop.${key}`) }}</b
          ><span>{{ t(`ecommerceLoop.loop.${key}Body`) }}</span>
        </div>
        <div class="ec-principle">
          <b>{{ t('ecommerceLoop.loop.principle') }}</b
          ><span>{{ t('ecommerceLoop.loop.ceoPrinciple') }}</span>
        </div></template
      >
    </button>
    <button
      v-if="launch"
      class="ec-launch-products ec-panel ec-tone-working"
      :class="{
        'ec-dim': productsDimmed,
        'ec-launch-products--preview': progress < 3,
      }"
      :inert="productsDimmed"
      :aria-label="t('ecommerceLoop.referenceFocus.products')"
      @click="emit('reference-focus', 'products')"
    >
      <div class="ec-product-pair">
        <div
          v-for="(product, index) in ['grapefruit', 'lime']"
          :key="product"
          class="ec-product-mini"
          :class="`ec-tone-${index === 0 ? 'danger' : 'lime'}`"
        >
          <ReferenceIcon kind="candle" :tone="index === 0 ? 'danger' : 'lime'" />
          <div>
            <b>{{ t(`ecommerceLoop.products.${product}`) }}</b
            ><small>{{ t('ecommerceLoop.launch.productSubtitle') }}</small>
          </div>
        </div>
      </div>
      <div class="ec-progress-chips">
        <span v-for="(key, index) in PROGRESS_KEYS" :key="key" :class="{ 'is-complete': index <= progress }">{{
          t(`ecommerceLoop.progress.${key}`)
        }}</span>
      </div>
      <p class="ec-launch-week" :class="{ 'ec-muted': progress < 7 }">
        {{ t('ecommerceLoop.launch.week') }}
      </p>
    </button>
  </section>
</template>
