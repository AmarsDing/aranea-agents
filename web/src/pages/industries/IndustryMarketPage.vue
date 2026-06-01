<template>
  <q-page padding>
    <div class="text-h4 q-mb-md">行业模板库</div>
    <div class="text-subtitle1 text-grey q-mb-lg">选择一个行业，安装完整的岗位级 Agent 团队</div>

    <div v-if="loading" class="row justify-center q-pa-lg">
      <q-spinner-dots size="40px" />
    </div>

    <div v-else-if="error" class="column items-center q-pa-lg">
      <q-icon name="error_outline" size="48px" color="negative" />
      <div class="text-subtitle1 q-mt-sm text-negative">{{ error }}</div>
      <q-btn flat label="重试" color="primary" class="q-mt-sm" @click="fetchIndustries" />
    </div>

    <div v-else-if="industries.length === 0" class="column items-center q-pa-lg">
      <q-icon name="business" size="48px" color="grey" />
      <div class="text-subtitle1 q-mt-sm text-grey">暂无行业模板</div>
    </div>

    <div v-else class="row q-col-gutter-md">
      <div v-for="ind in industries" :key="ind.key" class="col-12 col-md-4">
        <IndustryCard :industry="ind" @select="navigateToDetail" />
      </div>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import IndustryCard from '../../components/industries/IndustryCard.vue'
import { useIndustryMarket } from '../../features/industries/useIndustryMarket'
import type { Industry } from '../../features/industries/types'

const router = useRouter()
const { industries, loading, error, fetchIndustries } = useIndustryMarket()

function navigateToDetail(ind: Industry) {
  void router.push({ name: 'industry-detail', params: { key: ind.key } })
}

onMounted(() => { void fetchIndustries() })
</script>
