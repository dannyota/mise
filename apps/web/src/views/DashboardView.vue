<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { apiGet } from '@/api/client.js';
import type { DashboardSummary } from '@mise/contract';
import SummaryTile from '@/components/SummaryTile.vue';
import IngestStatus from '@/components/IngestStatus.vue';
import LoadingState from '@/components/LoadingState.vue';
import ErrorBanner from '@/components/ErrorBanner.vue';
import {
  FileText,
  AlertTriangle,
  CheckCircle,
  Clock,
} from 'lucide-vue-next';

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
          label="Total Documents"
          :value="data.total_documents"
          :icon="FileText"
        />
        <SummaryTile
          label="Open Findings"
          :value="data.open_findings"
          :icon="AlertTriangle"
        />
        <SummaryTile
          label="Resolved"
          :value="data.resolved_findings"
          :icon="CheckCircle"
        />
        <SummaryTile
          label="Pending Reviews"
          :value="data.pending_reviews"
          :icon="Clock"
        />
      </div>
      <IngestStatus :corpora="data.corpora" />
    </template>
  </div>
</template>
