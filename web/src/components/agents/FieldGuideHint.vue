<template>
  <div v-if="guide" class="field-guide-hint">
    <!-- Trigger icon -->
    <q-icon name="help_outline" size="18px" class="field-guide-hint__icon text-grey-6 cursor-pointer">
      <q-popup-proxy :offset="[0, 8]" max-width="340px">
        <q-card flat bordered class="field-guide-card q-pa-md">
          <div class="text-subtitle2 text-primary q-mb-xs">{{ guide.titleZh }}</div>
          <div class="text-caption text-grey-7 q-mb-sm">{{ guide.purpose }}</div>

          <div v-if="guide.shouldWrite.length" class="q-mb-sm">
            <div class="text-overline text-green-8">✓ 该写</div>
            <ul class="q-pl-md q-mb-none">
              <li v-for="item in guide.shouldWrite" :key="item" class="text-caption">{{ item }}</li>
            </ul>
          </div>

          <div v-if="guide.shouldAvoid.length" class="q-mb-sm">
            <div class="text-overline text-negative">✗ 不该写</div>
            <ul class="q-pl-md q-mb-none">
              <li v-for="item in guide.shouldAvoid" :key="item" class="text-caption">{{ item }}</li>
            </ul>
          </div>

          <div v-if="guide.budget.soft" class="text-caption text-grey-6 q-mt-xs">
            字符预算：<strong>{{ guide.budget.soft }}</strong> 软上限
            <template v-if="guide.budget.hard">
              / <strong>{{ guide.budget.hard }}</strong> 硬上限
            </template>
          </div>

          <div v-if="guide.examples.length" class="q-mt-sm">
            <div class="text-overline">示例</div>
            <q-expansion-item
              v-for="ex in guide.examples"
              :key="ex.industry"
              dense
              :label="`（${ex.industry}）`"
              class="text-caption"
            >
              <pre class="example-body q-pa-xs">{{ ex.body }}</pre>
            </q-expansion-item>
          </div>
        </q-card>
      </q-popup-proxy>
    </q-icon>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { FieldScope } from '../../features/agents/fieldGuides';
import { getFieldGuide } from '../../features/agents/fieldGuides';

const props = defineProps<{
  scope: FieldScope;
  fileName?: string;
}>();

const guide = computed(() => getFieldGuide(props.scope, props.fileName));
</script>

<style scoped>
.field-guide-hint {
  display: inline-flex;
  align-items: center;
}

.field-guide-hint__icon {
  vertical-align: middle;
}

.field-guide-card {
  max-width: 340px;
}

.example-body {
  font-size: var(--text-xs);
  background: var(--glass-surface);
  border-radius: 4px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
