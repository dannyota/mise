<script setup lang="ts">
import type { Finding } from '@mise/contract';
import { useRouter } from 'vue-router';

defineProps<{ items: Finding[] }>();
const router = useRouter();
</script>

<template>
  <div class="overflow-x-auto rounded border">
    <table class="w-full text-left text-sm">
      <thead class="border-b bg-gray-50">
        <tr>
          <th class="px-4 py-2 font-medium text-gray-600">Kind</th>
          <th class="px-4 py-2 font-medium text-gray-600">Description</th>
          <th class="px-4 py-2 font-medium text-gray-600">Severity</th>
          <th class="px-4 py-2 font-medium text-gray-600">Status</th>
        </tr>
      </thead>
      <tbody class="divide-y">
        <tr
          v-for="f in items"
          :key="f.id"
          class="cursor-pointer hover:bg-gray-50"
          @click="router.push(`/findings/${f.id}`)"
        >
          <td class="px-4 py-2 capitalize">{{ f.kind }}</td>
          <td class="max-w-xs truncate px-4 py-2 text-gray-600">{{ f.description }}</td>
          <td class="px-4 py-2">
            <span
              class="rounded px-2 py-0.5 text-xs"
              :class="{
                'bg-red-100 text-red-700': f.severity === 'critical' || f.severity === 'high',
                'bg-yellow-100 text-yellow-700': f.severity === 'medium',
                'bg-blue-100 text-blue-700': f.severity === 'low',
              }"
            >
              {{ f.severity }}
            </span>
          </td>
          <td class="px-4 py-2">{{ f.status }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
