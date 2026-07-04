<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { apiGet } from '@/api/client.js';
import type { CursorPage, Finding } from '@mise/contract';
import FindingsTable from '@/components/FindingsTable.vue';
import LoadingState from '@/components/LoadingState.vue';
import ErrorBanner from '@/components/ErrorBanner.vue';
import EmptyState from '@/components/EmptyState.vue';

const items = ref<Finding[]>([]);
const loading = ref(true);
const error = ref<Error | null>(null);

onMounted(async () => {
  try {
    const page = await apiGet<CursorPage<Finding>>('/findings');
    items.value = [...page.items];
  } catch (e) {
    error.value = e instanceof Error ? e : new Error(String(e));
  } finally {
    loading.value = false;
  }
});
</script>

<template>
  <div>
    <h2 class="mb-4 text-xl font-semibold">Findings</h2>
    <LoadingState v-if="loading" />
    <ErrorBanner v-else-if="error" :error="error" />
    <EmptyState v-else-if="!items.length" message="No findings." />
    <FindingsTable v-else :items="items" />
  </div>
</template>
