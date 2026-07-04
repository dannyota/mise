<script setup lang="ts">
import { Loader2 } from 'lucide-vue-next';
</script>

<template>
  <div
    class="flex items-center justify-center py-12"
    role="status"
  >
    <Loader2 class="animate-spin text-gray-400" :size="24" />
    <span class="sr-only">Loading...</span>
  </div>
</template>
