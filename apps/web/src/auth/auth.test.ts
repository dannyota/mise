import { beforeEach, describe, expect, it } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { useAuthStore } from './store.js';
import { useAuth } from './useAuth.js';

describe('auth store', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('starts unauthenticated', () => {
    const store = useAuthStore();
    expect(store.isAuthenticated).toBe(false);
    expect(store.token).toBeNull();
  });

  it('setToken authenticates', () => {
    const store = useAuthStore();
    store.setToken('abc');
    expect(store.isAuthenticated).toBe(true);
    expect(store.token).toBe('abc');
  });

  it('logout clears state', () => {
    const store = useAuthStore();
    store.setToken('abc');
    store.setSub('user1');
    store.logout();
    expect(store.isAuthenticated).toBe(false);
    expect(store.sub).toBeNull();
  });

  it('setCapabilities updates capabilities', () => {
    const store = useAuthStore();
    store.setCapabilities({
      translate_allowed: false,
      admin_allowed: true,
    });
    expect(store.capabilities.translate_allowed).toBe(false);
    expect(store.capabilities.admin_allowed).toBe(true);
  });
});

describe('useAuth composable', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('exposes reactive auth state', () => {
    const store = useAuthStore();
    const auth = useAuth();
    expect(auth.isAuthenticated.value).toBe(false);
    store.setToken('tok');
    expect(auth.isAuthenticated.value).toBe(true);
  });
});
