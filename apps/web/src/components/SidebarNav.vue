<script setup lang="ts">
import { useRoute } from 'vue-router';
import {
  LayoutDashboard,
  GitBranch,
  ClipboardCheck,
  AlertTriangle,
  MessageSquare,
  Clock,
  Bell,
  FileSpreadsheet,
  Settings,
} from 'lucide-vue-next';

const route = useRoute();

const links = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/graph', label: 'Graph', icon: GitBranch },
  { to: '/review', label: 'Review', icon: ClipboardCheck },
  { to: '/findings', label: 'Findings', icon: AlertTriangle },
  { to: '/chat', label: 'Audit Q&A', icon: MessageSquare },
  { to: '/timeline', label: 'Timeline', icon: Clock },
  {
    to: '/notifications',
    label: 'Notifications',
    icon: Bell,
  },
  { to: '/reports', label: 'Reports', icon: FileSpreadsheet },
  { to: '/admin', label: 'Admin', icon: Settings },
] as const;
</script>

<template>
  <nav class="bg-mise-sidebar flex w-56 flex-col gap-1 p-3 text-sm text-white">
    <h1 class="mb-4 px-3 text-lg font-bold">mise</h1>
    <RouterLink
      v-for="link in links"
      :key="link.to"
      :to="link.to"
      class="flex items-center gap-2 rounded px-3 py-2 hover:bg-white/10"
      :class="{
        'bg-white/20': route.path === link.to,
      }"
    >
      <component :is="link.icon" :size="18" />
      {{ link.label }}
    </RouterLink>
  </nav>
</template>
