import { computed } from 'vue';
import { useAuthStore } from './store.js';

export function useAuth() {
  const store = useAuthStore();

  return {
    token: computed(() => store.token),
    sub: computed(() => store.sub),
    tier: computed(() => store.tier),
    capabilities: computed(() => store.capabilities),
    isAuthenticated: computed(() => store.isAuthenticated),
    logout: () => store.logout(),
  };
}
