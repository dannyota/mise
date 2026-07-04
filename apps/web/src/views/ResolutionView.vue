<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { useRoute } from 'vue-router';
import { apiGet, apiPost } from '@/api/client.js';
import type { FindingDetail, Resolution } from '@mise/contract';
import ResolutionForm from '@/components/ResolutionForm.vue';
import EvidencePane from '@/components/EvidencePane.vue';
import LoadingState from '@/components/LoadingState.vue';
import ErrorBanner from '@/components/ErrorBanner.vue';

const route = useRoute();
const detail = ref<FindingDetail | null>(null);
const resolutions = ref<Resolution[]>([]);
const loading = ref(true);
const error = ref<Error | null>(null);

onMounted(async () => {
  try {
    const id = route.params.id as string;
    detail.value = await apiGet<FindingDetail>(`/findings/${id}`);
    resolutions.value = [...detail.value.resolutions];
  } catch (e) {
    error.value = e instanceof Error ? e : new Error(String(e));
  } finally {
    loading.value = false;
  }
});

async function handleResolve(note: string): Promise<void> {
  if (!detail.value) return;
  const resolution = await apiPost<Resolution>(
    `/findings/${detail.value.id}/resolve`,
    { note },
  );
  resolutions.value.push(resolution);
}
</script>

<template>
  <div>
    <LoadingState v-if="loading" />
    <ErrorBanner v-else-if="error" :error="error" />
    <template v-else-if="detail">
      <h2 class="mb-2 text-xl font-semibold capitalize">{{ detail.kind }}</h2>
      <p class="mb-4 text-sm text-gray-600">{{ detail.description }}</p>
      <div class="mb-4 flex gap-2">
        <span
          class="rounded px-2 py-0.5 text-xs"
          :class="{
            'bg-red-100 text-red-700':
              detail.severity === 'critical' || detail.severity === 'high',
            'bg-yellow-100 text-yellow-700': detail.severity === 'medium',
            'bg-blue-100 text-blue-700': detail.severity === 'low',
          }"
        >
          {{ detail.severity }}
        </span>
        <span class="text-sm text-gray-500">{{ detail.status }}</span>
      </div>
      <div class="mb-6">
        <EvidencePane
          :source-text="detail.source_text"
          :source-citation="detail.citation_path"
          :source-url="''"
          :target-text="detail.target_text"
          :target-citation="detail.target_citation_path"
        />
      </div>
      <div v-if="resolutions.length" class="mb-6">
        <h3 class="mb-2 text-sm font-semibold">Resolutions</h3>
        <div class="space-y-2">
          <div
            v-for="r in resolutions"
            :key="r.id"
            class="rounded border p-3 text-sm"
          >
            <div class="flex items-center justify-between">
              <span class="font-medium capitalize">{{ r.disposition }}</span>
              <span class="text-xs text-gray-500">{{ r.status }}</span>
            </div>
            <p v-if="r.notes" class="mt-1 text-gray-600">{{ r.notes }}</p>
          </div>
        </div>
      </div>
      <div>
        <h3 class="mb-2 text-sm font-semibold">Add Resolution</h3>
        <ResolutionForm @submit="handleResolve" />
      </div>
    </template>
  </div>
</template>
