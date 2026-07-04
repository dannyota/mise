import type { Config } from '../config.js';

export function shouldAbstain(supportScore: number, config: Config): boolean {
  return supportScore < config.ABSTAIN_THRESHOLD;
}

export function shouldEscalate(supportScore: number, config: Config): boolean {
  return supportScore < config.ESCALATION_THRESHOLD;
}
