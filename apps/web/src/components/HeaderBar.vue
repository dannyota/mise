<script setup lang="ts">
import { useAuth } from '@/auth/useAuth.js';
import { LogOut } from 'lucide-vue-next';

const { sub, tier, logout } = useAuth();
</script>

<template>
  <header class="flex h-12 items-center justify-between border-b bg-white px-4">
    <span class="text-sm text-gray-500">{{ tier }}</span>
    <div class="flex items-center gap-3">
      <span class="text-sm">{{ sub ?? 'dev-user' }}</span>
      <button
        class="rounded p-1 hover:bg-gray-100"
        title="Logout"
        @click="logout"
      >
        <LogOut :size="16" />
      </button>
    </div>
  </header>
</template>
