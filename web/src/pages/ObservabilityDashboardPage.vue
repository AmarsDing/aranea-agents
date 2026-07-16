<template>
  <q-page class="app-standard-page observability-page">
    <div class="observability-page__shell">
      <div class="observability-hero q-pa-md">
        <div class="text-caption text-grey-7">{{ t('menu.observability') }}</div>
        <div class="text-h5 text-weight-medium q-mt-xs">{{ t('observabilityPage.title') }}</div>
        <div class="text-caption text-grey-7 q-mt-xs">{{ t('observabilityPage.subtitle') }}</div>
      </div>

      <div class="observability-quick-links q-mt-md row q-gutter-sm">
        <q-btn outline no-caps rounded color="primary" icon="groups" :label="t('observabilityPage.teamRunsLink')" to="/team" />
        <q-btn outline no-caps rounded color="primary" icon="hub" :label="t('observabilityPage.graphExecutionsLink')" to="/graphs" />
        <q-btn
          outline
          no-caps
          rounded
          color="primary"
          icon="monitor_heart"
          :label="t('observabilityPage.metricsLink')"
          to="/monitor/logs"
        />
        <q-btn
          outline
          no-caps
          rounded
          color="primary"
          icon="terminal"
          :label="t('observabilityPage.flowLogsLink')"
          to="/monitor/logs"
        />
      </div>

      <div class="observability-panels q-mt-md">
          <div class="row q-gutter-sm items-center q-mb-md">
            <q-input
              v-model="sessionIdInput"
              outlined
              dense
              clearable
              :label="t('observabilityPage.sessionIdLabel')"
              :hint="t('observabilityPage.sessionIdHint')"
              class="observability-session-input"
              @keyup.enter="loadPlans"
            />
            <q-btn
              unelevated
              no-caps
              color="accent"
              icon="search"
              :label="t('observabilityPage.loadPlans')"
              :loading="plansLoading"
              @click="loadPlans"
            />
            <q-btn
              outline
              no-caps
              icon="clear"
              :label="t('observabilityPage.clearPlans')"
              :disable="plans.length === 0 && !selectedPlan"
              @click="clearPlans"
            />
          </div>

          <div class="row q-gutter-md">
            <div class="observability-plans-list-col">
              <q-card flat bordered>
                <q-card-section v-if="plansLoading" class="text-center q-pa-lg">
                  <q-spinner size="32px" />
                  <div class="text-caption text-grey-7 q-mt-sm">{{ t('common.loading') }}</div>
                </q-card-section>
                <q-card-section v-else-if="plans.length === 0" class="text-center text-grey-7 q-pa-lg">
                  {{ t('observabilityPage.noPlansFound') }}
                </q-card-section>
                <q-list v-else separator>
                  <q-item
                    v-for="plan in plans"
                    :key="plan.planId"
                    v-ripple
                    clickable
                    :active="selectedPlan?.planId === plan.planId"
                    active-class="bg-primary text-white"
                    @click="loadPlanDetail(plan.planId)"
                  >
                    <q-item-section>
                      <q-item-label lines="2">{{ plan.userMessage || plan.planId }}</q-item-label>
                      <q-item-label caption lines="1">
                        {{ t('observabilityPage.traceId') }}: {{ plan.traceId || '-' }}
                      </q-item-label>
                      <q-item-label caption lines="1">
                        {{ t('observabilityPage.createdAt') }}: {{ plan.createdAt || '-' }}
                      </q-item-label>
                    </q-item-section>
                    <q-item-section side top>
                      <div class="row q-gutter-xs">
                        <q-badge :color="complexityColor(plan.complexityLevel)" :label="plan.complexityLevel || '-'" />
                        <q-badge :color="statusColor(plan.status)" :label="plan.status || '-'" />
                      </div>
                      <div class="text-caption q-mt-xs">
                        {{ t('observabilityPage.strategy') }}: {{ plan.strategy || '-' }}
                      </div>
                      <div class="text-caption">{{ t('observabilityPage.subtaskCount') }}: {{ plan.subtaskCount }}</div>
                    </q-item-section>
                  </q-item>
                </q-list>
              </q-card>
            </div>

            <div class="observability-plan-detail-col">
              <q-card v-if="planDetailLoading" flat bordered>
                <q-card-section class="text-center q-pa-lg">
                  <q-spinner size="32px" />
                  <div class="text-caption text-grey-7 q-mt-sm">{{ t('common.loading') }}</div>
                </q-card-section>
              </q-card>
              <q-card v-else-if="!selectedPlan" flat bordered>
                <q-card-section class="text-center text-grey-7 q-pa-lg">
                  {{ t('observabilityPage.selectPlanToView') }}
                </q-card-section>
              </q-card>
              <q-card v-else flat bordered>
                <q-card-section>
                  <div class="text-h6">{{ t('observabilityPage.planDetail') }}</div>
                  <div class="text-caption text-grey-7 q-mt-xs">
                    {{ t('observabilityPage.traceId') }}: {{ selectedPlan.traceId || '-' }}
                  </div>
                </q-card-section>
                <q-separator />

                <q-card-section>
                  <div class="row q-gutter-sm items-center">
                    <div class="text-caption text-grey-7">{{ t('observabilityPage.userMessage') }}:</div>
                    <div class="text-body2">{{ selectedPlan.userMessage || '-' }}</div>
                  </div>
                  <div class="row q-gutter-sm items-center q-mt-xs">
                    <div class="text-caption text-grey-7">{{ t('observabilityPage.complexity') }}:</div>
                    <q-badge
                      :color="complexityColor(selectedPlan.complexityLevel)"
                      :label="selectedPlan.complexityLevel || '-'"
                    />
                    <div class="text-caption text-grey-7">
                      {{ t('observabilityPage.complexityScore') }}: {{ selectedPlan.complexityScore }}
                    </div>
                  </div>
                  <div class="row q-gutter-sm items-center q-mt-xs">
                    <div class="text-caption text-grey-7">{{ t('observabilityPage.strategy') }}:</div>
                    <div class="text-body2">{{ selectedPlan.strategy || '-' }}</div>
                    <q-badge :color="statusColor(selectedPlan.status)" :label="selectedPlan.status || '-'" />
                  </div>
                  <div class="row q-gutter-sm items-center q-mt-xs">
                    <div class="text-caption text-grey-7">{{ t('observabilityPage.createdAt') }}:</div>
                    <div class="text-body2">{{ selectedPlan.createdAt || '-' }}</div>
                    <div class="text-caption text-grey-7 q-ml-sm">{{ t('observabilityPage.updatedAt') }}:</div>
                    <div class="text-body2">{{ selectedPlan.updatedAt || '-' }}</div>
                  </div>
                </q-card-section>
                <q-separator />

                <q-expansion-item
                  v-if="selectedPlan.dimensions"
                  :label="t('observabilityPage.dimensions')"
                  icon="analytics"
                  default-opened
                >
                  <q-card-section class="q-pl-lg">
                    <div class="row q-gutter-sm">
                      <div class="observability-dim-item">
                        <div class="text-caption text-grey-7">{{ t('observabilityPage.dimSemantic') }}</div>
                        <div class="text-body2 text-weight-medium">{{ selectedPlan.dimensions.semantic }}</div>
                      </div>
                      <div class="observability-dim-item">
                        <div class="text-caption text-grey-7">{{ t('observabilityPage.dimStructural') }}</div>
                        <div class="text-body2 text-weight-medium">{{ selectedPlan.dimensions.structural }}</div>
                      </div>
                      <div class="observability-dim-item">
                        <div class="text-caption text-grey-7">{{ t('observabilityPage.dimDomain') }}</div>
                        <div class="text-body2 text-weight-medium">{{ selectedPlan.dimensions.domain }}</div>
                      </div>
                      <div class="observability-dim-item">
                        <div class="text-caption text-grey-7">{{ t('observabilityPage.dimTool') }}</div>
                        <div class="text-body2 text-weight-medium">{{ selectedPlan.dimensions.tool }}</div>
                      </div>
                      <div class="observability-dim-item">
                        <div class="text-caption text-grey-7">{{ t('observabilityPage.dimContext') }}</div>
                        <div class="text-body2 text-weight-medium">{{ selectedPlan.dimensions.context }}</div>
                      </div>
                      <div class="observability-dim-item">
                        <div class="text-caption text-grey-7">{{ t('observabilityPage.dimHistorical') }}</div>
                        <div class="text-body2 text-weight-medium">{{ selectedPlan.dimensions.historical }}</div>
                      </div>
                    </div>
                  </q-card-section>
                </q-expansion-item>
                <q-separator />

                <q-expansion-item
                  :label="t('observabilityPage.subTasks')"
                  icon="list_alt"
                  :caption="String(selectedPlan.subTasks.length)"
                  default-opened
                >
                  <q-list separator dense>
                    <q-item v-for="sub in selectedPlan.subTasks" :key="sub.id">
                      <q-item-section>
                        <q-item-label>{{ sub.name || sub.id }}</q-item-label>
                        <q-item-label caption lines="2">{{ sub.description || '-' }}</q-item-label>
                        <q-item-label caption>
                          {{ t('observabilityPage.subTaskDependsOn') }}:
                          {{ sub.dependsOn.length ? sub.dependsOn.join(', ') : '-' }}
                        </q-item-label>
                      </q-item-section>
                      <q-item-section side top>
                        <div class="text-caption">{{ t('observabilityPage.subTaskPriority') }}: {{ sub.priority }}</div>
                        <div class="text-caption">
                          {{ t('observabilityPage.subTaskComplexity') }}: {{ sub.estimatedComplexity }}
                        </div>
                      </q-item-section>
                    </q-item>
                  </q-list>
                </q-expansion-item>
                <q-separator />

                <q-expansion-item
                  v-if="selectedPlan.strategyReason || selectedPlan.decomposeReason || selectedPlan.topologyHint"
                  :label="t('observabilityPage.strategyReason')"
                  icon="lightbulb"
                >
                  <q-card-section>
                    <div v-if="selectedPlan.strategyReason" class="q-mb-sm">
                      <div class="text-caption text-grey-7">{{ t('observabilityPage.strategyReason') }}</div>
                      <div class="text-body2">{{ selectedPlan.strategyReason }}</div>
                    </div>
                    <div v-if="selectedPlan.decomposeReason" class="q-mb-sm">
                      <div class="text-caption text-grey-7">{{ t('observabilityPage.decomposeReason') }}</div>
                      <div class="text-body2">{{ selectedPlan.decomposeReason }}</div>
                    </div>
                    <div v-if="selectedPlan.topologyHint">
                      <div class="text-caption text-grey-7">{{ t('observabilityPage.topologyHint') }}</div>
                      <div class="text-body2">{{ selectedPlan.topologyHint }}</div>
                    </div>
                  </q-card-section>
                </q-expansion-item>
                <q-separator v-if="selectedPlan.memoryHit" />

                <q-expansion-item v-if="selectedPlan.memoryHit" :label="t('observabilityPage.memoryHit')" icon="memory">
                  <q-card-section>
                    <div class="row q-gutter-sm items-center q-mb-xs">
                      <div class="text-caption text-grey-7">{{ t('observabilityPage.memoryCacheId') }}:</div>
                      <div class="text-body2">{{ selectedPlan.memoryHit.cacheId || '-' }}</div>
                    </div>
                    <div class="row q-gutter-sm items-center q-mb-xs">
                      <div class="text-caption text-grey-7">{{ t('observabilityPage.memoryDqScore') }}:</div>
                      <div class="text-body2">{{ selectedPlan.memoryHit.dqScore }}</div>
                    </div>
                    <div class="row q-gutter-sm items-center q-mb-xs">
                      <div class="text-caption text-grey-7">{{ t('observabilityPage.memoryTopologyUsed') }}:</div>
                      <div class="text-body2">{{ selectedPlan.memoryHit.topologyUsed || '-' }}</div>
                    </div>
                    <div class="row q-gutter-sm items-center">
                      <div class="text-caption text-grey-7">{{ t('observabilityPage.memoryAgentKeys') }}:</div>
                      <div class="text-body2">
                        {{
                          selectedPlan.memoryHit.agentKeysUsed.length
                            ? selectedPlan.memoryHit.agentKeysUsed.join(', ')
                            : '-'
                        }}
                      </div>
                    </div>
                  </q-card-section>
                </q-expansion-item>
                <q-separator v-if="selectedPlan.intentArtifactJson" />

                <q-expansion-item
                  v-if="selectedPlan.intentArtifactJson"
                  :label="t('observabilityPage.intentArtifact')"
                  icon="description"
                >
                  <q-card-section>
                    <pre class="observability-json-pre">{{ selectedPlan.intentArtifactJson }}</pre>
                  </q-card-section>
                </q-expansion-item>
              </q-card>
            </div>
          </div>
      </div>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { useObservabilityDashboard } from '../features/observability/useObservabilityDashboard';

const {
  sessionIdInput,
  plansLoading,
  plans,
  selectedPlan,
  planDetailLoading,
  loadPlans,
  loadPlanDetail,
  clearPlans,
  t,
} = useObservabilityDashboard();

function statusColor(status: string): string {
  switch (status) {
    case 'draft':
      return 'grey';
    case 'confirmed':
      return 'positive';
    case 'executing':
      return 'blue';
    case 'completed':
      return 'positive';
    case 'failed':
      return 'negative';
    case 'abandoned':
      return 'warning';
    default:
      return 'grey';
  }
}

function complexityColor(level: string): string {
  switch (level) {
    case 'simple':
      return 'green';
    case 'moderate':
      return 'orange';
    case 'complex':
      return 'red';
    default:
      return 'grey';
  }
}
</script>

<style lang="sass" scoped>
.observability-page__shell
  max-width: 1400px
  margin: 0 auto

.observability-hero
  padding-bottom: 0

.observability-quick-links
  padding: 0 16px

.observability-panels
  padding: 0 16px

.observability-session-input
  min-width: 320px
  flex: 1
  max-width: 560px

.observability-plans-list-col
  flex: 1
  min-width: 320px
  max-width: 560px

.observability-plan-detail-col
  flex: 1
  min-width: 320px

.observability-dim-item
  min-width: 96px

.observability-json-pre
  white-space: pre-wrap
  word-break: break-word
  font-family: 'SF Mono', 'Monaco', 'Consolas', monospace
  font-size: 0.8rem
  background: var(--color-code-block-bg)
  padding: 8px
  border-radius: 6px
  margin: 0
</style>
