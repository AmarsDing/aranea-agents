import { defineStore } from 'pinia';
import { ref } from 'vue';
import { loginAdminByUsername, loginAdminByEmail, logoutAdmin, getCurrentAdmin } from '../../features/admin/api';
import type { AdminSession } from '../../features/admin/types';

export const useAdminStore = defineStore('admin', () => {
  const currentAdmin = ref<AdminSession | null>(null);
  const loading = ref(false);

  async function loginByUsername(username: string, password: string) {
    loading.value = true;
    try {
      currentAdmin.value = await loginAdminByUsername(username, password);
      return currentAdmin.value;
    } finally {
      loading.value = false;
    }
  }

  async function loginByEmail(email: string, password: string) {
    loading.value = true;
    try {
      currentAdmin.value = await loginAdminByEmail(email, password);
      return currentAdmin.value;
    } finally {
      loading.value = false;
    }
  }

  async function logout() {
    await logoutAdmin();
    currentAdmin.value = null;
  }

  async function fetchCurrentAdmin() {
    currentAdmin.value = await getCurrentAdmin();
    return currentAdmin.value;
  }

  return { currentAdmin, loading, loginByUsername, loginByEmail, logout, fetchCurrentAdmin };
});
