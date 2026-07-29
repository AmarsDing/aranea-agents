<template>
  <div>
    <AppPageToolbar dense>
      <span class="text-caption text-grey-7">{{ t('a2a.federation.orgFieldDomainHint') }}</span>
      <template #actions>
        <q-btn
          unelevated
          no-caps
          color="primary"
          icon="add"
          :label="t('a2a.federation.orgRegister')"
          @click="registerOpen = true"
        />
      </template>
    </AppPageToolbar>

    <AppRegistryTable
      row-key="id"
      :rows="orgs"
      :columns="columns"
      :loading="loading"
      hide-pagination
      :pagination="{ rowsPerPage: 0 }"
      :no-data-label="t('a2a.federation.orgEmpty')"
    >
      <template #body-cell-trust_level="props">
        <q-td :props="props">
          <q-badge
            :color="federationTrustColor(props.row.trust_level)"
            :label="federationTrustLabel(t, props.row.trust_level)"
          />
        </q-td>
      </template>
      <template #body-cell-auth_type="props">
        <q-td :props="props">
          {{ a2aAuthTypeLabel(props.row.auth_type) }}
          <span v-if="props.row.auth_type !== 'none'" class="text-caption text-grey-6">
            ·
            {{
              props.row.auth_config_set
                ? t('a2a.federation.orgAuthConfigured')
                : t('a2a.federation.orgAuthNotConfigured')
            }}
          </span>
        </q-td>
      </template>
      <template #body-cell-status="props">
        <q-td :props="props">
          <q-badge
            :color="federationOrgStatusColor(props.row.status)"
            :label="federationOrgStatusLabel(t, props.row.status)"
          />
        </q-td>
      </template>
      <template #body-cell-actions="props">
        <q-td :props="props">
          <div class="app-registry-cell-actions">
            <q-btn
              flat
              dense
              round
              icon="edit"
              color="primary"
              :aria-label="t('a2a.federation.orgEditTrust')"
              @click="openTrustDialog(props.row)"
            >
              <q-tooltip>{{ t('a2a.federation.orgEditTrust') }}</q-tooltip>
            </q-btn>
            <q-btn
              flat
              dense
              round
              icon="sync"
              color="primary"
              :loading="syncingOrgId === props.row.id"
              :aria-label="t('a2a.federation.orgSyncCards')"
              @click="emit('sync', props.row)"
            >
              <q-tooltip>{{ t('a2a.federation.orgSyncCards') }}</q-tooltip>
            </q-btn>
            <q-btn
              flat
              dense
              round
              icon="delete"
              color="negative"
              :aria-label="t('a2a.federation.orgDelete')"
              @click="emit('remove', props.row)"
            >
              <q-tooltip>{{ t('a2a.federation.orgDelete') }}</q-tooltip>
            </q-btn>
          </div>
        </q-td>
      </template>
    </AppRegistryTable>

    <!-- 注册组织 Dialog -->
    <q-dialog v-model="registerOpen" persistent>
      <q-card class="app-dialog-card app-glass-dialog app-dialog-card--lg">
        <q-card-section class="app-glass-dialog__head row items-center justify-between">
          <div class="app-glass-dialog__title">{{ t('a2a.federation.orgRegisterTitle') }}</div>
          <q-btn flat round dense icon="close" :disable="registerLoading" @click="registerOpen = false" />
        </q-card-section>
        <q-card-section class="app-glass-dialog__scroll">
          <div class="app-form-field-grid">
            <q-input
              v-model.trim="registerForm.name"
              dense
              outlined
              :label="t('a2a.federation.orgFieldName') + ' *'"
              :disable="registerLoading"
            />
            <q-input
              v-model.trim="registerForm.domain"
              dense
              outlined
              :label="t('a2a.federation.orgFieldDomain') + ' *'"
              :hint="t('a2a.federation.orgFieldDomainHint')"
              :disable="registerLoading"
            />
            <q-input
              v-model.trim="registerForm.public_base_url"
              dense
              outlined
              :label="t('a2a.federation.orgFieldPublicBaseUrl')"
              class="app-grid-span-full"
              :disable="registerLoading"
            />
            <q-select
              v-model="registerForm.trust_level"
              dense
              outlined
              emit-value
              map-options
              :options="trustOptions"
              :label="t('a2a.federation.orgFieldTrustLevel')"
              :disable="registerLoading"
            />
            <q-select
              v-model="registerForm.auth_type"
              dense
              outlined
              emit-value
              map-options
              :options="authTypeOptions"
              :label="t('a2a.federation.orgFieldAuthType')"
              :disable="registerLoading"
            />
            <q-input
              v-if="registerForm.auth_type === 'api_key' || registerForm.auth_type === 'bearer'"
              v-model="authSecret"
              dense
              outlined
              class="app-grid-span-full"
              :type="showSecret ? 'text' : 'password'"
              :label="registerForm.auth_type === 'bearer' ? 'Bearer Token' : 'API Key'"
              :disable="registerLoading"
            >
              <template #append>
                <q-btn
                  flat
                  dense
                  round
                  :icon="showSecret ? 'visibility_off' : 'visibility'"
                  @click="showSecret = !showSecret"
                />
              </template>
            </q-input>
            <template v-if="registerForm.auth_type === 'mtls'">
              <q-input
                v-model="mtls.cert_file"
                dense
                outlined
                :label="t('a2a.federation.orgFieldCertFile')"
                :disable="registerLoading"
              />
              <q-input
                v-model="mtls.key_file"
                dense
                outlined
                :label="t('a2a.federation.orgFieldKeyFile')"
                :disable="registerLoading"
              />
              <q-input
                v-model="mtls.ca_file"
                dense
                outlined
                :label="t('a2a.federation.orgFieldCaFile')"
                class="app-grid-span-full"
                :disable="registerLoading"
              />
            </template>
          </div>
        </q-card-section>
        <q-card-actions align="right" class="q-px-md q-pb-md">
          <q-btn
            flat
            rounded
            no-caps
            :label="t('common.cancel')"
            :disable="registerLoading"
            @click="registerOpen = false"
          />
          <q-btn
            color="primary"
            unelevated
            rounded
            no-caps
            :label="t('a2a.federation.orgRegister')"
            :loading="registerLoading"
            @click="onRegisterSubmit"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- 信任等级编辑 Dialog -->
    <q-dialog :model-value="trustTarget !== null" persistent @update:model-value="onTrustDialogToggle">
      <q-card class="app-dialog-card app-glass-dialog">
        <q-card-section class="app-glass-dialog__head row items-center justify-between">
          <div class="app-glass-dialog__title">{{ t('a2a.federation.orgEditTrust') }} · {{ trustTarget?.name }}</div>
          <q-btn flat round dense icon="close" @click="trustTarget = null" />
        </q-card-section>
        <q-card-section>
          <q-select
            v-model="trustDraft"
            dense
            outlined
            emit-value
            map-options
            :options="trustOptions"
            :label="t('a2a.federation.orgFieldTrustLevel')"
          />
        </q-card-section>
        <q-card-actions align="right" class="q-px-md q-pb-md">
          <q-btn flat rounded no-caps :label="t('common.cancel')" @click="trustTarget = null" />
          <q-btn color="primary" unelevated rounded no-caps :label="t('common.confirm')" @click="onTrustSubmit" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageToolbar from '../../layout/AppPageToolbar.vue';
import AppRegistryTable from '../../layout/AppRegistryTable.vue';
import type { FederationOrg, RegisterFederationOrgInput } from '../../../features/a2a/federationTypes';
import {
  federationOrgStatusColor,
  federationOrgStatusLabel,
  federationTrustColor,
  federationTrustLabel,
  federationTrustOptions,
} from '../../../features/a2a/federationUi';
import { a2aAuthTypeLabel } from '../../../features/a2a/a2aTableUi';
import { A2A_AUTH_TYPE_OPTIONS, buildA2AAuthJSON } from '../../../features/a2a/authUtils';
import type { RegistryTableColumn } from '../../../features/ui/registryTableColumns';

defineProps<{
  orgs: FederationOrg[];
  loading: boolean;
  registerLoading: boolean;
  syncingOrgId: string;
  columns: RegistryTableColumn<FederationOrg>[];
}>();

const emit = defineEmits<{
  register: [payload: RegisterFederationOrgInput];
  setTrust: [payload: { id: string; trust_level: string }];
  sync: [org: FederationOrg];
  remove: [org: FederationOrg];
}>();

const { t } = useI18n();

const registerOpen = ref(false);
const showSecret = ref(false);
const authSecret = ref('');
const registerForm = reactive({
  name: '',
  domain: '',
  public_base_url: '',
  trust_level: 'neutral',
  auth_type: 'none',
});
const mtls = reactive({ cert_file: '', key_file: '', ca_file: '' });

const trustTarget = ref<FederationOrg | null>(null);
const trustDraft = ref('neutral');

const trustOptions = computed(() => federationTrustOptions(t));
const authTypeOptions = A2A_AUTH_TYPE_OPTIONS;

function onRegisterSubmit() {
  emit('register', {
    name: registerForm.name.trim(),
    domain: registerForm.domain.trim(),
    public_base_url: registerForm.public_base_url.trim(),
    trust_level: registerForm.trust_level,
    auth_type: registerForm.auth_type,
    auth_config_json: buildA2AAuthJSON(registerForm.auth_type, authSecret.value, mtls) ?? '',
  });
}

/** 注册成功后由父级调用：关闭 Dialog 并重置表单。 */
function closeRegisterDialog() {
  registerOpen.value = false;
  registerForm.name = '';
  registerForm.domain = '';
  registerForm.public_base_url = '';
  registerForm.trust_level = 'neutral';
  registerForm.auth_type = 'none';
  authSecret.value = '';
  mtls.cert_file = '';
  mtls.key_file = '';
  mtls.ca_file = '';
}

function openTrustDialog(org: FederationOrg) {
  trustTarget.value = org;
  trustDraft.value = org.trust_level || 'neutral';
}

function onTrustDialogToggle(v: boolean) {
  if (!v) trustTarget.value = null;
}

function onTrustSubmit() {
  if (!trustTarget.value) return;
  emit('setTrust', { id: trustTarget.value.id, trust_level: trustDraft.value });
  trustTarget.value = null;
}

defineExpose({ closeRegisterDialog });
</script>
