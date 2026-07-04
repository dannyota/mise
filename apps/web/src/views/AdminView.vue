<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { apiGet, apiPost } from '@/api/client.js';
import type { CorpusAdmin } from '@mise/contract';
import LoadingState from '@/components/LoadingState.vue';
import ErrorBanner from '@/components/ErrorBanner.vue';
import { RefreshCw } from 'lucide-vue-next';

const corpora = ref<CorpusAdmin[]>([]);
const loading = ref(true);
const error = ref<Error | null>(null);

onMounted(async () => {
  try {
    corpora.value = await apiGet<CorpusAdmin[]>('/admin/corpora');
  } catch (e) {
    error.value = e instanceof Error ? e : new Error(String(e));
  } finally {
    loading.value = false;
  }
});

async function retryIngest(corpusId: string): Promise<void> {
  await apiPost(`/admin/corpora/${corpusId}/retry`);
}
</script>

<template>
  <div>
    <h2 class="mb-4 text-xl font-semibold">Corpus Administration</h2>
    <LoadingState v-if="loading" />
    <ErrorBanner v-else-if="error" :error="error" />
    <div v-else class="space-y-3">
      <div
        v-for="c in corpora"
        :key="c.corpus_id"
        class="flex items-center justify-between rounded border p-4"
      >
        <div>
          <p class="font-medium">{{ c.corpus_id }}</p>
          <p class="text-xs text-gray-500">{{ c.document_count }} documents</p>
        </div>
        <div class="flex items-center gap-2">
          <span
            class="rounded px-2 py-0.5 text-xs"
            :class="{
              'bg-green-100 text-green-700': c.status === 'healthy',
              'bg-yellow-100 text-yellow-700': c.status === 'ingesting',
              'bg-red-100 text-red-700': c.status === 'error',
            }"
          >
            {{ c.status }}
          </span>
          <button
            v-if="c.status === 'error'"
            class="rounded p-1 text-gray-400 hover:text-violet-700"
            title="Retry ingest"
            @click="retryIngest(c.corpus_id)"
          >
            <RefreshCw :size="16" />
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
