<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { apiGet } from '@/api/client.js';
import type { CursorPage, TimelineEvent } from '@mise/contract';
import ResolutionTimeline from '@/components/ResolutionTimeline.vue';
import LoadingState from '@/components/LoadingState.vue';
import ErrorBanner from '@/components/ErrorBanner.vue';
import EmptyState from '@/components/EmptyState.vue';

const events = ref<TimelineEvent[]>([]);
const loading = ref(true);
const error = ref<Error | null>(null);

onMounted(async () => {
  try {
    const page = await apiGet<CursorPage<TimelineEvent>>('/timeline');
    events.value = [...page.items];
  } catch (e) {
    error.value = e instanceof Error ? e : new Error(String(e));
  } finally {
    loading.value = false;
  }
});
</script>

<template>
  <div>
    <h2 class="mb-4 text-xl font-semibold">Audit Timeline</h2>
    <LoadingState v-if="loading" />
    <ErrorBanner v-else-if="error" :error="error" />
    <EmptyState v-else-if="!events.length" message="No timeline events." />
    <ResolutionTimeline v-else :events="events" />
  </div>
</template>
