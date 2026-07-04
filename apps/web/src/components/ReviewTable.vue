<script setup lang="ts">
import {
  useVueTable,
  getCoreRowModel,
  createColumnHelper,
  FlexRender,
} from '@tanstack/vue-table';
import type { ReviewCandidate } from '@mise/contract';

const props = defineProps<{ items: ReviewCandidate[] }>();
const emit = defineEmits<{
  accept: [id: string];
  reject: [id: string];
  relink: [id: string];
}>();

const col = createColumnHelper<ReviewCandidate>();

const columns = [
  col.accessor('source_corpus_id', { header: 'Source' }),
  col.accessor('target_corpus_id', { header: 'Target' }),
  col.accessor('confidence', {
    header: 'Confidence',
    cell: (info) => `${(info.getValue() * 100).toFixed(0)}%`,
  }),
  col.accessor('grounding_score', {
    header: 'Grounding',
    cell: (info) => `${(info.getValue() * 100).toFixed(0)}%`,
  }),
  col.accessor('status', { header: 'Status' }),
  col.display({
    id: 'actions',
    header: 'Actions',
    cell: (info) => info.row.original.edge_id,
  }),
];

const table = useVueTable({
  get data() {
    return props.items;
  },
  columns,
  getCoreRowModel: getCoreRowModel(),
});
</script>

<template>
  <div class="overflow-x-auto rounded border">
    <table class="w-full text-left text-sm">
      <thead class="border-b bg-gray-50">
        <tr
          v-for="headerGroup in table.getHeaderGroups()"
          :key="headerGroup.id"
        >
          <th
            v-for="header in headerGroup.headers"
            :key="header.id"
            class="px-4 py-2 font-medium text-gray-600"
          >
            <FlexRender
              :render="header.column.columnDef.header"
              :props="header.getContext()"
            />
          </th>
        </tr>
      </thead>
      <tbody class="divide-y">
        <tr v-for="row in table.getRowModel().rows" :key="row.id">
          <td
            v-for="cell in row.getVisibleCells()"
            :key="cell.id"
            class="px-4 py-2"
          >
            <template v-if="cell.column.id === 'actions'">
              <div class="flex gap-1">
                <button
                  class="rounded bg-green-100 px-2 py-0.5 text-xs text-green-700"
                  @click="emit('accept', row.original.edge_id)"
                >
                  Accept
                </button>
                <button
                  class="rounded bg-red-100 px-2 py-0.5 text-xs text-red-700"
                  @click="emit('reject', row.original.edge_id)"
                >
                  Reject
                </button>
                <button
                  class="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-700"
                  @click="emit('relink', row.original.edge_id)"
                >
                  Relink
                </button>
              </div>
            </template>
            <template v-else>
              <FlexRender
                :render="cell.column.columnDef.cell"
                :props="cell.getContext()"
              />
            </template>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
