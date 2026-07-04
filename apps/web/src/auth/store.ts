import { defineStore } from 'pinia';
import { ref, computed } from 'vue';

export type Capabilities = {
  translate_allowed: boolean;
  admin_allowed: boolean;
};

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string | null>(null);
  const sub = ref<string | null>(null);
  const tier = ref<string>('mise_public');
  const capabilities = ref<Capabilities>({
    translate_allowed: true,
    admin_allowed: false,
  });
  const isAuthenticated = computed(() => token.value !== null);

  function setToken(t: string | null): void {
    token.value = t;
  }
  function setSub(s: string): void {
    sub.value = s;
  }
  function setTier(t: string): void {
    tier.value = t;
  }
  function setCapabilities(c: Capabilities): void {
    capabilities.value = c;
  }
  function logout(): void {
    token.value = null;
    sub.value = null;
  }

  return {
    token,
    sub,
    tier,
    capabilities,
    isAuthenticated,
    setToken,
    setSub,
    setTier,
    setCapabilities,
    logout,
  };
});
