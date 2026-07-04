import {
  UserManager,
  WebStorageStateStore,
} from 'oidc-client-ts';

let manager: UserManager | null = null;

export function createUserManager(): UserManager | null {
  const issuer =
    typeof import.meta !== 'undefined'
      ? import.meta.env?.VITE_OIDC_ISSUER
      : undefined;
  if (!issuer) return null;

  const clientId =
    typeof import.meta !== 'undefined'
      ? (import.meta.env?.VITE_OIDC_CLIENT_ID ??
        'mise-web')
      : 'mise-web';

  manager = new UserManager({
    authority: issuer,
    client_id: clientId,
    redirect_uri: `${window.location.origin}/callback`,
    response_type: 'code',
    scope: 'openid profile email',
    userStore: new WebStorageStateStore({
      store: window.sessionStorage,
    }),
  });
  return manager;
}

export function getUserManager(): UserManager | null {
  return manager;
}
