<script setup lang="ts">
import { FileText } from 'lucide-vue-next';

defineProps<{ citation: string }>();
</script>

<template>
  <span
    class="inline-flex items-center gap-1 rounded bg-violet-100 px-2 py-0.5 text-xs text-violet-700"
  >
    <FileText :size="12" />
    {{ citation }}
  </span>
</template>
