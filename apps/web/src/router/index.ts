import { createRouter, createWebHistory } from 'vue-router';
import { authGuard } from '@/auth/guard.js';

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      component: () => import('@/views/LoginView.vue'),
    },
    {
      path: '/callback',
      component: () => import('@/views/CallbackView.vue'),
    },
    {
      path: '/',
      component: () => import('@/views/DashboardView.vue'),
    },
    {
      path: '/graph',
      component: () => import('@/views/GraphView.vue'),
    },
    {
      path: '/review',
      component: () => import('@/views/ReviewView.vue'),
    },
    {
      path: '/findings',
      component: () => import('@/views/FindingsView.vue'),
    },
    {
      path: '/findings/:id',
      component: () => import('@/views/ResolutionView.vue'),
    },
    {
      path: '/chat',
      component: () => import('@/views/ChatView.vue'),
    },
    {
      path: '/timeline',
      component: () => import('@/views/TimelineView.vue'),
    },
    {
      path: '/notifications',
      component: () => import('@/views/NotificationsView.vue'),
    },
    {
      path: '/reports',
      component: () => import('@/views/ReportsView.vue'),
    },
    {
      path: '/admin',
      component: () => import('@/views/AdminView.vue'),
    },
  ],
});

router.beforeEach(authGuard);

export default router;
