export type Caller = {
  readonly sub: string;
  readonly roles: readonly string[];
  readonly tier: string;
  readonly correlationId: string;
};

const CALLER_KEY = 'caller';

export function setCaller(
  c: { set: (key: string, value: unknown) => void },
  caller: Caller,
): void {
  c.set(CALLER_KEY, caller);
}

export function getCaller(c: { get: (key: string) => unknown }): Caller {
  const caller = c.get(CALLER_KEY);
  if (!caller) {
    throw new Error('caller not set — OIDC middleware missing');
  }
  return caller as Caller;
}
