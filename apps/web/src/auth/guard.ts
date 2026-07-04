import type {
  NavigationGuardNext,
  RouteLocationNormalized,
} from 'vue-router';
import { useAuthStore } from './store.js';

const PUBLIC_ROUTES = ['/login', '/callback'];

export function authGuard(
  to: RouteLocationNormalized,
  _from: RouteLocationNormalized,
  next: NavigationGuardNext,
): void {
  if (PUBLIC_ROUTES.includes(to.path)) {
    next();
    return;
  }
  const store = useAuthStore();
  if (!store.isAuthenticated) {
    next('/login');
    return;
  }
  if (
    to.path === '/admin' &&
    !store.capabilities.admin_allowed
  ) {
    next('/');
    return;
  }
  next();
}
