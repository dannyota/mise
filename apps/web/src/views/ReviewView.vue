<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { apiGet, apiPost } from '@/api/client.js';
import type { CursorPage, ReviewCandidate } from '@mise/contract';
import ReviewTable from '@/components/ReviewTable.vue';
import LoadingState from '@/components/LoadingState.vue';
import ErrorBanner from '@/components/ErrorBanner.vue';
import EmptyState from '@/components/EmptyState.vue';

const items = ref<ReviewCandidate[]>([]);
const loading = ref(true);
const error = ref<Error | null>(null);

onMounted(async () => {
  try {
    const page = await apiGet<CursorPage<ReviewCandidate>>('/reviews');
    items.value = [...page.items];
  } catch (e) {
    error.value = e instanceof Error ? e : new Error(String(e));
  } finally {
    loading.value = false;
  }
});

async function handleAccept(id: string): Promise<void> {
  await apiPost(`/reviews/${id}/accept`);
  items.value = items.value.filter((r) => r.edge_id !== id);
}

async function handleReject(id: string): Promise<void> {
  await apiPost(`/reviews/${id}/reject`);
  items.value = items.value.filter((r) => r.edge_id !== id);
}
</script>

<template>
  <div>
    <h2 class="mb-4 text-xl font-semibold">Review Workbench</h2>
    <LoadingState v-if="loading" />
    <ErrorBanner v-else-if="error" :error="error" />
    <EmptyState v-else-if="!items.length" message="No items pending review." />
    <ReviewTable
      v-else
      :items="items"
      @accept="handleAccept"
      @reject="handleReject"
      @relink="() => {}"
    />
  </div>
</template>
