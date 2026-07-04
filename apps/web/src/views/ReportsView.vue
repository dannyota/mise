<script setup lang="ts">
import { ref } from 'vue';
import { apiPost } from '@/api/client.js';
import { FileSpreadsheet, Download } from 'lucide-vue-next';
import ErrorBanner from '@/components/ErrorBanner.vue';

const exporting = ref(false);
const error = ref<Error | null>(null);

const formats = [
  { id: 'csv', label: 'CSV', mime: 'text/csv' },
  { id: 'json', label: 'JSON', mime: 'application/json' },
] as const;

async function exportReport(format: string): Promise<void> {
  exporting.value = true;
  error.value = null;
  try {
    const res = await apiPost<{ download_url: string }>('/reports/export', { format });
    window.open(res.download_url, '_blank');
  } catch (e) {
    error.value = e instanceof Error ? e : new Error(String(e));
  } finally {
    exporting.value = false;
  }
}
</script>

<template>
  <div>
    <h2 class="mb-4 text-xl font-semibold">Reports</h2>
    <ErrorBanner v-if="error" :error="error" class="mb-4" />
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
      <button
        v-for="f in formats"
        :key="f.id"
        :disabled="exporting"
        class="flex items-center gap-3 rounded border p-4 hover:bg-gray-50 disabled:opacity-50"
        @click="exportReport(f.id)"
      >
        <FileSpreadsheet :size="24" class="text-violet-700" />
        <div class="text-left">
          <p class="font-medium">Export {{ f.label }}</p>
          <p class="text-xs text-gray-500">Download findings report</p>
        </div>
        <Download :size="16" class="ml-auto text-gray-400" />
      </button>
    </div>
  </div>
</template>
