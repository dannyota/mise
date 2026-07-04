<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { apiGet, apiPost, apiDelete } from '@/api/client.js';
import type { Webhook } from '@mise/contract';
import { Trash2, Plus } from 'lucide-vue-next';
import ErrorBanner from './ErrorBanner.vue';

const webhooks = ref<Webhook[]>([]);
const newUrl = ref('');
const error = ref<Error | null>(null);

onMounted(async () => {
  try {
    webhooks.value = await apiGet<Webhook[]>('/webhooks');
  } catch (e) {
    error.value = e instanceof Error ? e : new Error(String(e));
  }
});

async function add(): Promise<void> {
  const url = newUrl.value.trim();
  if (!url) return;
  try {
    new URL(url);
  } catch {
    error.value = new Error('Invalid URL');
    return;
  }
  try {
    const hook = await apiPost<Webhook>('/webhooks', { url });
    webhooks.value.push(hook);
    newUrl.value = '';
    error.value = null;
  } catch (e) {
    error.value = e instanceof Error ? e : new Error(String(e));
  }
}

async function remove(id: string): Promise<void> {
  await apiDelete(`/webhooks/${id}`);
  webhooks.value = webhooks.value.filter((w) => w.id !== id);
}
</script>

<template>
  <div>
    <h3 class="mb-3 text-sm font-semibold">Webhooks</h3>
    <ErrorBanner v-if="error" :error="error" class="mb-3" />
    <div class="mb-3 flex gap-2">
      <input
        v-model="newUrl"
        type="url"
        placeholder="https://..."
        class="flex-1 rounded border px-3 py-1.5 text-sm focus:border-violet-500 focus:outline-none"
      />
      <button
        :disabled="!newUrl.trim()"
        class="flex items-center gap-1 rounded bg-violet-700 px-3 py-1.5 text-sm text-white disabled:opacity-50"
        @click="add"
      >
        <Plus :size="14" />
        Add
      </button>
    </div>
    <div class="space-y-2">
      <div
        v-for="w in webhooks"
        :key="w.id"
        class="flex items-center justify-between rounded border px-3 py-2 text-sm"
      >
        <span class="truncate">{{ w.url }}</span>
        <button
          class="shrink-0 rounded p-1 text-gray-400 hover:text-red-600"
          @click="remove(w.id)"
        >
          <Trash2 :size="14" />
        </button>
      </div>
    </div>
  </div>
</template>
