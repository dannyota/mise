<script setup lang="ts">
import { onMounted } from 'vue';
import { useNotificationsStore } from '@/stores/notifications.js';
import LoadingState from '@/components/LoadingState.vue';
import EmptyState from '@/components/EmptyState.vue';
import { CheckCircle } from 'lucide-vue-next';

const store = useNotificationsStore();

onMounted(() => store.fetch());
</script>

<template>
  <div>
    <h2 class="mb-4 text-xl font-semibold">Notifications</h2>
    <LoadingState v-if="store.loading" />
    <EmptyState v-else-if="!store.items.length" message="No notifications." />
    <div v-else class="space-y-2">
      <div
        v-for="n in store.items"
        :key="n.id"
        class="flex items-center justify-between rounded border p-3 text-sm"
        :class="n.read ? 'bg-white' : 'bg-violet-50'"
      >
        <div>
          <p class="font-medium">{{ n.title }}</p>
          <p class="text-xs text-gray-500">{{ n.created_at }}</p>
        </div>
        <button
          v-if="!n.read"
          class="rounded p-1 text-gray-400 hover:text-green-600"
          title="Mark as read"
          @click="store.markRead(n.id)"
        >
          <CheckCircle :size="16" />
        </button>
      </div>
    </div>
  </div>
</template>
