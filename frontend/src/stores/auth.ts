import { defineStore } from 'pinia'
import { ref } from 'vue'
import { http, type Envelope } from '@/api/http'
import type { User } from '@/types/models'

export const useAuth = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const ready = ref(false)

  async function fetchMe() {
    try {
      user.value = (await http.get<Envelope<User>>('/auth/me')).data
    } catch {
      user.value = null
    } finally {
      ready.value = true
    }
  }

  async function login(email: string, password: string) {
    const res = await http.post<Envelope<User>>('/auth/login', { email, password })
    user.value = res.data
    return res.message
  }

  async function logout() {
    try {
      await http.post('/auth/logout')
    } finally {
      user.value = null
    }
  }

  return { user, ready, fetchMe, login, logout }
})
