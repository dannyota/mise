<script setup lang="ts">
import type { TimelineEvent } from '@mise/contract';

defineProps<{ events: readonly TimelineEvent[] }>();
</script>

<template>
  <div class="space-y-3">
    <div
      v-for="evt in events"
      :key="evt.id"
      class="flex gap-3 border-l-2 border-violet-200 pl-4"
    >
      <div>
        <p class="text-sm font-medium">{{ evt.description }}</p>
        <p class="text-xs text-gray-500">
          {{ evt.kind }} &middot; {{ evt.timestamp }}
        </p>
      </div>
    </div>
  </div>
</template>
