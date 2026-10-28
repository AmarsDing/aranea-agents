<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { COMPANY_MODULES, RESOURCE_TONES, SHOPS } from '../../features/ecommerce-loop/data';
import type { ShopId } from '../../features/ecommerce-loop/types';
import ReferenceIcon from './ReferenceIcon.vue';
defineProps<{ focused: boolean; selectedShop: ShopId }>();
const emit = defineEmits<{
  (event: 'focus-commerce'): void;
  (event: 'open-loop'): void;
  (event: 'select-shop', shop: ShopId): void;
}>();
const { t } = useI18n();
</script>

<template>
  <section class="ec-company" :class="{ 'ec-company--focused': focused }">
    <div class="ec-company-title">
      <span class="ec-outline-tag">{{ t('ecommerceLoop.company.previous') }}</span>
      <h1>{{ t('ecommerceLoop.company.title') }}</h1>
      <p>
        {{ t('ecommerceLoop.company.subtitleBefore') }}<strong>{{ t('ecommerceLoop.company.subtitleAccent') }}</strong
        >{{ t('ecommerceLoop.company.subtitleAfter') }}
      </p>
    </div>
    <div class="ec-founder ec-panel">
      <span class="ec-founder-avatar">S</span>
      <div>
        Simon<small>{{ t('ecommerceLoop.company.founder') }}</small>
      </div>
    </div>
    <div class="ec-founder-links">
      <span>{{ t('ecommerceLoop.company.direction') }}</span
      ><span
        >{{ t('ecommerceLoop.company.report') }}<small>{{ t('ecommerceLoop.company.daily') }}</small></span
      >
    </div>
    <div class="ec-company-ceo ec-panel ec-tone-ceo">
      <ReferenceIcon />
      <div>
        CEO Agent<small>{{ t('ecommerceLoop.company.ceoRole') }}</small
        ><b>{{ t('ecommerceLoop.company.ceoHint') }}</b>
      </div>
    </div>
    <div class="ec-company-key">
      <p class="ec-tone-ceo"><i />{{ t('ecommerceLoop.company.down') }}</p>
      <p class="ec-tone-owner"><i />{{ t('ecommerceLoop.company.up') }}</p>
      <p class="ec-tone-working"><i />{{ t('ecommerceLoop.company.running') }}</p>
    </div>
    <svg class="ec-company-lines" viewBox="0 0 1920 1080" aria-hidden="true">
      <path class="ec-line ec-tone-owner" d="M940 175 C900 210 940 255 940 258" />
      <path class="ec-line ec-tone-ceo" d="M990 258 C1035 210 990 195 990 175" />
      <g
        v-for="(module, index) in COMPANY_MODULES"
        :key="module.id"
        :class="[`ec-tone-${module.tone}`, { 'ec-dim': focused && index !== 0 }]"
      >
        <path class="ec-line" :d="`M960 360 C960 411 ${350 + index * 245} 411 ${350 + index * 245} 464`" />
        <path
          class="ec-line ec-line--flow"
          :d="`M975 360 C975 425 ${365 + index * 245} 425 ${365 + index * 245} 464`"
        />
        <path
          v-for="(child, row) in module.children"
          :key="child"
          class="ec-line"
          :d="`M${249 + index * 245} 580 V${673 + row * 65} Q${249 + index * 245} ${673 + row * 65} ${264 + index * 245} ${673 + row * 65}`"
        />
      </g>
    </svg>
    <div
      v-for="(module, index) in COMPANY_MODULES"
      :key="module.id"
      class="ec-company-column"
      :class="[`ec-tone-${module.tone}`, { 'ec-dim': focused && index !== 0 }]"
      :style="{ left: `${239 + index * 245}px` }"
    >
      <div class="ec-branch-label" :style="{ left: `${260 - index * 71}px` }">
        <span v-if="module.id !== 'investment'">{{ t('ecommerceLoop.company.dispatch') }}</span
        ><small>{{ t('ecommerceLoop.company.dailyReport') }}</small>
      </div>
      <component
        :is="index === 0 ? 'button' : 'div'"
        class="ec-company-module ec-panel"
        :class="{
          'ec-running-border': index === 0,
          'ec-company-module--commerce': index === 0,
          'ec-company-module--knowledge': index === 4,
        }"
        :type="index === 0 ? 'button' : undefined"
        @click="index === 0 && emit('focus-commerce')"
      >
        <div class="ec-module-title">
          <q-icon :name="module.icon" />{{ t(`ecommerceLoop.modules.${module.id}.title`)
          }}<i class="ec-module-status" :class="index === 0 ? 'ec-tone-owner' : 'ec-tone-ai'" />
        </div>
        <div class="ec-module-job">
          <b>{{ t(`ecommerceLoop.modules.${module.id}.job`) }}</b
          ><span>{{ t(`ecommerceLoop.modules.${module.id}.scope`) }}</span>
        </div>
        <small>{{ t('ecommerceLoop.company.frequency') }}</small>
      </component>
      <div class="ec-company-resources">
        <component
          :is="index === 0 ? 'button' : 'div'"
          v-for="(child, row) in module.children"
          :key="child"
          class="ec-resource ec-panel"
          :type="index === 0 ? 'button' : undefined"
          :class="{ 'ec-resource--selected': index === 0 && SHOPS[row] === selectedShop }"
          @click="index === 0 && emit('select-shop', SHOPS[row])"
        >
          <i :class="`ec-tone-${RESOURCE_TONES[child] ?? module.tone}`" />
          <div>
            {{ t(`ecommerceLoop.children.${child}`)
            }}<small>{{
              ['otherPlatform', 'status', 'schedule'].includes(child)
                ? t(`ecommerceLoop.childNotes.${child}`)
                : t(`ecommerceLoop.modules.${module.id}.child`)
            }}</small>
          </div>
        </component>
      </div>
    </div>
    <button v-if="focused" class="ec-company-focus-label" @click="emit('open-loop')">
      {{ t('ecommerceLoop.company.focus') }}
    </button>
  </section>
</template>
