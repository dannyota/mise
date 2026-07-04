<script setup lang="ts">
import { computed } from 'vue';
import { ApiClientError } from '@/api/client.js';
import { AlertCircle } from 'lucide-vue-next';

const props = defineProps<{ error: Error }>();

const title = computed(() => {
  if (props.error instanceof ApiClientError) {
    return props.error.problem.title;
  }
  return props.error.message;
});

const detail = computed(() => {
  if (props.error instanceof ApiClientError) {
    return props.error.problem.detail;
  }
  return undefined;
});
</script>

<template>
  <div
    class="flex gap-3 rounded border border-red-200 bg-red-50 p-4"
    role="alert"
  >
    <AlertCircle class="shrink-0 text-red-500" :size="20" />
    <div>
      <p class="font-medium text-red-800">{{ title }}</p>
      <p v-if="detail" class="mt-1 text-sm text-red-600">
        {{ detail }}
      </p>
    </div>
  </div>
</template>
