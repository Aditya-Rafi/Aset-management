import { defineStore } from 'pinia'
import { ref } from 'vue'
import { http, type Envelope, type Paged } from '@/api/http'
import type { Category, InspectionTemplate } from '@/types/models'

export const useLookups = defineStore('lookups', () => {
  const categories = ref<Category[]>([])
  const loaded = ref(false)
  const publicBaseUrl = ref('')
  const baseConfigured = ref(false)
  const companyName = ref('')
  const templates = ref<Record<string, InspectionTemplate>>({})

  async function loadCategories(force = false) {
    if (loaded.value && !force) return
    categories.value = (await http.get<Paged<Category>>('/categories?per_page=100&sort=name&order=asc')).data
    loaded.value = true
  }

  async function loadConfig() {
    const c = (await http.get<Envelope<{ public_base_url: string; configured: boolean; company_name: string }>>('/config')).data
    publicBaseUrl.value = c.public_base_url
    baseConfigured.value = c.configured
    companyName.value = c.company_name
  }

  async function loadTemplates() {
    if (Object.keys(templates.value).length) return
    templates.value = (await http.get<Envelope<Record<string, InspectionTemplate>>>('/inspection-templates')).data
  }

  const byId = (id?: number | null) => categories.value.find((c) => c.id === id)
  return { categories, loaded, publicBaseUrl, baseConfigured, companyName, templates, loadCategories, loadConfig, loadTemplates, byId }
})
