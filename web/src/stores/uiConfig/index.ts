import { defineStore } from 'pinia';
import { ref } from 'vue';

const STORAGE_KEY = 'chat.ui.showToolCalls';

export const useUiConfigStore = defineStore('uiConfig', () => {
  const showToolCalls = ref(localStorage.getItem(STORAGE_KEY) !== 'false');

  function setShowToolCalls(v: boolean): void {
    showToolCalls.value = v;
    localStorage.setItem(STORAGE_KEY, String(v));
  }

  return { showToolCalls, setShowToolCalls };
});
