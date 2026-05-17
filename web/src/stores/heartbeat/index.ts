import { defineStore } from "pinia";
import { ref } from "vue";

/** Tracks WebSocket heartbeat / server-alive state shared across components. */
export const useHeartbeatStore = defineStore("heartbeat", () => {
  const isAlive = ref(false);
  const lastPongAt = ref<number | null>(null);
  const reconnectCount = ref(0);

  function onPong() {
    isAlive.value = true;
    lastPongAt.value = Date.now();
  }

  function onDisconnect() {
    isAlive.value = false;
    reconnectCount.value++;
  }

  function reset() {
    isAlive.value = false;
    lastPongAt.value = null;
    reconnectCount.value = 0;
  }

  return { isAlive, lastPongAt, reconnectCount, onPong, onDisconnect, reset };
});
