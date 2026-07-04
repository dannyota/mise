<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { apiGet } from '@/api/client.js';
import type { DashboardSummary } from '@mise/contract';
import SummaryTile from '@/components/SummaryTile.vue';
import IngestStatus from '@/components/IngestStatus.vue';
import LoadingState from '@/components/LoadingState.vue';
import ErrorBanner from '@/components/ErrorBanner.vue';
import { ShieldCheck, AlertTriangle, Bell, ClipboardCheck } from 'lucide-vue-next';

const data = ref<DashboardSummary | null>(null);
const error = ref<Error | null>(null);
const loading = ref(true);

onMounted(async () => {
  try {
    data.value = await apiGet<DashboardSummary>('/dashboard');
  } catch (e) {
    error.value = e instanceof Error ? e : new Error(String(e));
  } finally {
    loading.value = false;
  }
});
</script>

<template>
  <div>
    <h2 class="mb-6 text-xl font-semibold">Dashboard</h2>
    <LoadingState v-if="loading" />
    <ErrorBanner v-else-if="error" :error="error" />
    <template v-else-if="data">
      <div class="mb-6 grid grid-cols-2 gap-4 lg:grid-cols-4">
        <SummaryTile
          label="Coverage"
          :value="`${data.coverage_pct}%`"
          :icon="ShieldCheck"
        />
        <SummaryTile
          label="Open Conflicts"
          :value="data.open_conflicts"
          :icon="AlertTriangle"
        />
        <SummaryTile
          label="Staleness Alerts"
          :value="data.staleness_alerts"
          :icon="Bell"
        />
        <SummaryTile
          label="Review Queue"
          :value="data.review_queue_depth"
          :icon="ClipboardCheck"
        />
      </div>
      <IngestStatus :corpora="data.corpora" />
    </template>
  </div>
</template>
