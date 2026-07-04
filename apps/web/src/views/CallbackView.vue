<script setup lang="ts">
import { onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { getUserManager } from '@/auth/oidc.js';
import { useAuthStore } from '@/auth/store.js';
import { setBearerToken } from '@/api/client.js';
import { apiGet } from '@/api/client.js';
import type { BootstrapResponse } from '@mise/contract';

const router = useRouter();
const store = useAuthStore();

onMounted(async () => {
  const manager = getUserManager();
  if (!manager) {
    await router.push('/');
    return;
  }
  const user = await manager.signinCallback();
  if (user?.access_token) {
    store.setToken(user.access_token);
    store.setSub(user.profile.sub);
    setBearerToken(user.access_token);
    const boot = await apiGet<BootstrapResponse>('/bootstrap');
    store.setTier(boot.tier);
    store.setCapabilities(boot.capabilities);
  }
  await router.push('/');
});
</script>

<template>
  <div class="flex h-screen items-center justify-center">
    <p class="text-gray-500">Completing login...</p>
  </div>
</template>
