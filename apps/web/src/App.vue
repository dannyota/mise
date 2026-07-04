<script setup lang="ts">
import { RouterView } from 'vue-router';
import SidebarNav from '@/components/SidebarNav.vue';
import HeaderBar from '@/components/HeaderBar.vue';
import { useAuth } from '@/auth/useAuth.js';

const { isAuthenticated } = useAuth();
</script>

<template>
  <div v-if="isAuthenticated" class="flex h-screen">
    <SidebarNav />
    <div class="flex flex-1 flex-col overflow-hidden">
      <HeaderBar />
      <main class="flex-1 overflow-y-auto p-6">
        <RouterView />
      </main>
    </div>
  </div>
  <RouterView v-else />
</template>
