import { createRemoteJWKSet, jwtVerify } from 'jose';
import type { MiddlewareHandler } from 'hono';
import { setCaller } from './caller.js';
import type { Caller } from './caller.js';
import type { Config } from '../config.js';

export function oidcMiddleware(config: Config): MiddlewareHandler {
  if (!config.OIDC_ISSUER) {
    return fakeCaller();
  }

  const jwks = createRemoteJWKSet(
    new URL('.well-known/jwks.json', config.OIDC_ISSUER),
  );

  return async (c, next) => {
    const auth = c.req.header('Authorization');
    if (!auth?.startsWith('Bearer ')) {
      return c.json(
        { type: 'unauthorized', title: 'Missing bearer token', status: 401 },
        401,
      );
    }
    const token = auth.slice(7);

    try {
      const { payload } = await jwtVerify(token, jwks, {
        issuer: config.OIDC_ISSUER,
        audience: config.OIDC_AUDIENCE,
      });

      const caller: Caller = {
        sub: payload.sub ?? 'unknown',
        roles: Array.isArray(payload.roles) ? (payload.roles as string[]) : [],
        tier: typeof payload.tier === 'string' ? payload.tier : 'mise_public',
        correlationId: c.req.header('X-Correlation-Id') ?? crypto.randomUUID(),
      };
      setCaller(c, caller);
    } catch {
      return c.json(
        { type: 'unauthorized', title: 'Invalid token', status: 401 },
        401,
      );
    }

    await next();
  };
}

function fakeCaller(): MiddlewareHandler {
  return async (c, next) => {
    const caller: Caller = {
      sub: c.req.header('X-Fake-Sub') ?? 'dev-user',
      roles: (c.req.header('X-Fake-Roles') ?? 'analyst').split(','),
      tier: c.req.header('X-Fake-Tier') ?? 'mise_local',
      correlationId: c.req.header('X-Correlation-Id') ?? crypto.randomUUID(),
    };
    setCaller(c, caller);
    await next();
  };
}
