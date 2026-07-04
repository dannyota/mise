<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { apiGet } from '@/api/client.js';
import type { GraphResponse, ChainResponse } from '@mise/contract';
import GraphCanvas from '@/components/GraphCanvas.vue';
import ChainDrawer from '@/components/ChainDrawer.vue';
import LoadingState from '@/components/LoadingState.vue';
import ErrorBanner from '@/components/ErrorBanner.vue';

const graph = ref<GraphResponse | null>(null);
const chain = ref<ChainResponse | null>(null);
const loading = ref(true);
const error = ref<Error | null>(null);
const drawerOpen = ref(false);

onMounted(async () => {
  try {
    graph.value = await apiGet<GraphResponse>('/graph');
  } catch (e) {
    error.value = e instanceof Error ? e : new Error(String(e));
  } finally {
    loading.value = false;
  }
});

async function handleSelectNode(id: string): Promise<void> {
  try {
    chain.value = await apiGet<ChainResponse>(`/graph/chain/${id}`);
    drawerOpen.value = true;
  } catch (e) {
    error.value = e instanceof Error ? e : new Error(String(e));
  }
}
</script>

<template>
  <div class="relative flex h-full flex-col">
    <h2 class="mb-4 text-xl font-semibold">Regulatory Graph</h2>
    <LoadingState v-if="loading" />
    <ErrorBanner v-else-if="error" :error="error" class="mb-4" />
    <div v-else-if="graph" class="relative flex-1">
      <GraphCanvas
        :nodes="graph.nodes"
        :edges="graph.edges"
        @select-node="handleSelectNode"
      />
      <ChainDrawer
        v-if="chain"
        :hops="chain.hops"
        :open="drawerOpen"
        @close="drawerOpen = false"
      />
    </div>
  </div>
</template>
