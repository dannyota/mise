<script setup lang="ts">
import { onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { getUserManager } from '@/auth/oidc.js';
import { useAuthStore } from '@/auth/store.js';
import { setBearerToken } from '@/api/client.js';

const router = useRouter();
const store = useAuthStore();

onMounted(async () => {
  const manager = getUserManager();
  if (!manager) {
    store.setToken('dev-token');
    store.setSub('dev-user');
    setBearerToken('dev-token');
    await router.push('/');
    return;
  }
  await manager.signinRedirect();
});
</script>

<template>
  <div class="flex h-screen items-center justify-center">
    <p class="text-gray-500">Redirecting to login...</p>
  </div>
</template>
