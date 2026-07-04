import { ref, type Ref } from 'vue';
import { apiGet } from '@/api/client.js';
import type { CursorPage } from '@mise/contract';

export function usePagination<T>(path: string) {
  const items: Ref<T[]> = ref([]);
  const cursor = ref<string | null>(null);
  const loading = ref(false);
  const hasMore = ref(true);

  async function loadMore(): Promise<void> {
    if (loading.value || !hasMore.value) return;
    loading.value = true;
    try {
      const params: Record<string, string> = {};
      if (cursor.value) params.cursor = cursor.value;
      const page = await apiGet<CursorPage<T>>(path, params);
      items.value = [...items.value, ...page.items] as T[];
      cursor.value = page.cursor;
      hasMore.value = page.cursor !== null;
    } finally {
      loading.value = false;
    }
  }

  function reset(): void {
    items.value = [];
    cursor.value = null;
    hasMore.value = true;
  }

  return { items, loading, hasMore, loadMore, reset };
}
