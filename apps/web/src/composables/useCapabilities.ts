import { ref, computed } from 'vue';
import { useAuthStore } from '@/auth/store.js';
import { apiGet } from '@/api/client.js';
import type { BootstrapResponse } from '@mise/contract';

export function useCapabilities() {
  const store = useAuthStore();
  const loaded = ref(false);

  const tier = computed(() => store.tier);
  const translateAllowed = computed(() => store.capabilities.translate_allowed);
  const adminAllowed = computed(() => store.capabilities.admin_allowed);

  async function refresh(): Promise<void> {
    const boot = await apiGet<BootstrapResponse>('/bootstrap');
    store.setTier(boot.tier);
    store.setCapabilities(boot.capabilities);
    loaded.value = true;
  }

  return { tier, translateAllowed, adminAllowed, loaded, refresh };
}
