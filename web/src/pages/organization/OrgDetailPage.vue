<template>
  <q-page padding>
    <q-btn
      flat
      icon="arrow_back"
      label="公司模板库"
      class="q-mb-md"
      @click="router.push({ name: 'org-market' })"
    />

    <div v-if="loading" class="row justify-center q-pa-lg">
      <q-spinner-dots size="40px" />
    </div>

    <template v-else-if="company">
      <div class="row items-center q-mb-lg">
        <span class="text-h3 q-mr-md">{{ company.icon || company.name?.charAt(0) || '🏢' }}</span>
        <div>
          <div class="text-h4">{{ company.name }}</div>
          <div class="text-subtitle1 text-grey">{{ company.description }}</div>
        </div>
      </div>

      <div class="text-h5 q-mb-md">部门</div>
      <q-list bordered separator>
        <q-expansion-item
          v-for="dep in departments"
          :key="dep.key"
          :label="dep.name"
          :caption="dep.description"
          @show="fetchPositions(dep.key)"
        >
          <q-card flat>
            <q-card-section>
              <div v-if="!departmentPositions[dep.key]" class="text-grey">加载中...</div>
              <draggable
                v-else
                v-model="departmentPositions[dep.key]"
                item-key="key"
                ghost-class="position-item--ghost"
                chosen-class="position-item--chosen"
                drag-class="position-item--dragging"
                :animation="200"
                :delay="100"
                @update:model-value="(val: Position[]) => reorderPositions(dep.key, val)"
              >
                <template #item="{ element: pos }">
                  <q-item>
                    <q-item-section>
                      <q-item-label>{{ pos.name }}</q-item-label>
                      <q-item-label caption>{{ pos.description }}</q-item-label>
                      <q-item-label v-if="pos.seniority_level" caption>职级：{{ pos.seniority_level }}</q-item-label>
                    </q-item-section>
                  </q-item>
                </template>
              </draggable>
            </q-card-section>
          </q-card>
        </q-expansion-item>
      </q-list>
    </template>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import draggable from 'vuedraggable';
import { useOrgDetail } from '../../features/organization/useOrgDetail';
import type { Position } from '../../features/organization/types';

const router = useRouter();
const route = useRoute();
const companyKey = route.params.key as string;
const { company, departments, departmentPositions, loading, fetchDetail, fetchPositions, reorderPositions } =
  useOrgDetail(companyKey);

onMounted(() => {
  void fetchDetail();
});
</script>
