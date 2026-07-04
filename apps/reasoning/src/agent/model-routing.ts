import type { Config } from '../config.js';

export function selectModel(supportScore: number, config: Config): string {
  if (supportScore < config.ESCALATION_THRESHOLD) {
    return config.MODEL_ESCALATION;
  }
  return config.MODEL_DEFAULT;
}
