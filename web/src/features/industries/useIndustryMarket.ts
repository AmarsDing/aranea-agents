import { ref } from 'vue'
import { listIndustries } from './api'
import type { Industry } from './types'

export function useIndustryMarket() {
  const industries = ref<Industry[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetchIndustries() {
    loading.value = true
    error.value = null
    try {
      const result = await listIndustries()
      industries.value = result.items
    } catch (e: any) {
      error.value = e?.message ?? 'Failed to load industries'
    } finally {
      loading.value = false
    }
  }

  return { industries, loading, error, fetchIndustries }
}
