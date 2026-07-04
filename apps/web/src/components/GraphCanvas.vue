<script setup lang="ts">
import { ref, watch } from 'vue';
import { VueFlow, useVueFlow } from '@vue-flow/core';
import '@vue-flow/core/dist/style.css';
import '@vue-flow/core/dist/theme-default.css';
import ELK from 'elkjs/lib/elk.bundled.js';
import type { RestGraphNode, RestGraphEdge } from '@mise/contract';

const props = defineProps<{
  nodes: RestGraphNode[];
  edges: RestGraphEdge[];
}>();

const emit = defineEmits<{ selectNode: [id: string] }>();

const { fitView } = useVueFlow();

const flowNodes = ref<
  { id: string; position: { x: number; y: number }; data: { label: string } }[]
>([]);
const flowEdges = ref<{ id: string; source: string; target: string }[]>([]);

const elk = new ELK();

async function layout(): Promise<void> {
  const graph = await elk.layout({
    id: 'root',
    children: props.nodes.map((n) => ({
      id: n.id,
      width: 180,
      height: 40,
    })),
    edges: props.edges.map((e) => ({
      id: e.id,
      sources: [e.source],
      targets: [e.target],
    })),
    layoutOptions: {
      'elk.algorithm': 'layered',
      'elk.direction': 'RIGHT',
      'elk.spacing.nodeNode': '60',
    },
  });

  flowNodes.value = (graph.children ?? []).map((c) => ({
    id: c.id,
    position: { x: c.x ?? 0, y: c.y ?? 0 },
    data: { label: props.nodes.find((n) => n.id === c.id)?.label ?? c.id },
  }));

  flowEdges.value = props.edges.map((e) => ({
    id: e.id,
    source: e.source,
    target: e.target,
  }));

  setTimeout(() => fitView(), 50);
}

watch(() => [props.nodes, props.edges], layout, { immediate: true });
</script>

<template>
  <VueFlow
    :nodes="flowNodes"
    :edges="flowEdges"
    :default-viewport="{ zoom: 1, x: 0, y: 0 }"
    class="h-full w-full"
    @node-click="(e) => emit('selectNode', e.node.id)"
  />
</template>
