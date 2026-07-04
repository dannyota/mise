import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import { apiGet, apiPost } from '@/api/client.js';
import type { CursorPage, Notification } from '@mise/contract';

export const useNotificationsStore = defineStore('notifications', () => {
  const items = ref<Notification[]>([]);
  const loading = ref(false);

  const unreadCount = computed(() => items.value.filter((n) => !n.read).length);

  async function fetch(): Promise<void> {
    loading.value = true;
    try {
      const page = await apiGet<CursorPage<Notification>>('/notifications');
      items.value = [...page.items];
    } finally {
      loading.value = false;
    }
  }

  async function markRead(id: string): Promise<void> {
    await apiPost(`/notifications/${id}/read`);
    const item = items.value.find((n) => n.id === id);
    if (item) {
      (item as { read: boolean }).read = true;
    }
  }

  return { items, loading, unreadCount, fetch, markRead };
});
