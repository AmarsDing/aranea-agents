<template>
  <section class="settings-section">
    <div class="section-heading">
      <div>
        <div class="text-subtitle1 text-weight-bold">A2A 端点</div>
        <div class="text-caption text-grey-7">将本 Agent 暴露为 A2A 服务，供同工作区 call_agent 或外部客户端调用。</div>
      </div>
    </div>

    <q-inner-loading :showing="loading" />

    <div v-if="card" class="q-gutter-md">
      <q-toggle :model-value="card.enabled" color="primary" label="启用 A2A" @update:model-value="setCardEnabled" />
      <template v-if="card.enabled">
        <div class="app-form-field-grid app-form-field-grid--2col">
          <q-input
            :model-value="card.endpoint_url || ''"
            class="app-field-long"
            dense
            outlined
            readonly
            :label="$t('agentSettings.a2aEndpointUrl')"
            :hint="
              card.endpoint_url ? $t('agentSettings.a2aEndpointUrlHint') : $t('agentSettings.a2aEndpointUrlNoBase')
            "
          >
            <template v-if="card.endpoint_url" #append>
              <q-btn flat round dense icon="content_copy" @click="copyEndpointUrl">
                <q-tooltip>{{ $t('agentSettings.a2aCopyUrl') }}</q-tooltip>
              </q-btn>
            </template>
          </q-input>
          <q-input
            :model-value="$t('agentSettings.a2aStreamingValue')"
            dense
            outlined
            readonly
            :label="$t('agentSettings.a2aStreamingLabel')"
            :hint="$t('agentSettings.a2aStreamingHint')"
          />
        </div>
        <div>
          <q-btn
            flat
            rounded
            no-caps
            color="primary"
            icon="travel_explore"
            :label="$t('agentSettings.a2aDiscoverLink')"
            to="/a2a"
          />
        </div>
      </template>
      <div>
        <div class="text-caption text-grey-7 q-mb-sm">{{ $t('agentSettings.a2aCapabilitiesLabel') }}</div>
        <q-input
          :model-value="capabilityLines"
          class="app-field-long"
          outlined
          type="textarea"
          rows="4"
          :hint="$t('agentSettings.a2aCapabilitiesHint')"
          @update:model-value="capabilityLines = String($event ?? '')"
        />
      </div>
      <div class="app-actions-bar app-actions-bar--start">
        <q-btn
          color="primary"
          rounded
          unelevated
          no-caps
          :label="$t('agentSettings.a2aSaveCard')"
          :loading="saving"
          :disable="!card"
          @click="saveEndpoint"
        />
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
// Container: approved — A2A endpoint Tab；内部调用 useAgentA2AEndpointTab。
import { useAgentA2AEndpointTab } from '../../features/agents/useAgentA2AEndpointTab';

const props = defineProps<{
  agentId: string;
}>();

// 禁止用 reactive() 包裹 composable 返回值：解构 reactive 对象会当场解包 ref，
// 丢失响应性（card 永远停留在初始 null，Tab 永久空白）。
const { loading, saving, card, capabilityLines, setCardEnabled, saveEndpoint, copyEndpointUrl } =
  useAgentA2AEndpointTab(() => props.agentId);
</script>
