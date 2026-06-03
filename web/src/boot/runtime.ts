import { defineBoot } from '#q-app/wrappers';
import { loadRuntimeConfig } from '../config/runtime';
import { syncHttpClients } from '../services/axiosHandler';

export default defineBoot(async () => {
  await loadRuntimeConfig();
  syncHttpClients();
});
