import { z } from 'zod';

const envSchema = z.object({
  PORT: z.coerce.number().default(3001),
  SERVING_URL: z.string().default('http://localhost:8080'),
  NODE_ENV: z
    .enum(['development', 'production', 'test'])
    .default('development'),
  MODEL_DEFAULT: z.string().default('claude-haiku-4-5-20251001'),
  MODEL_ESCALATION: z.string().default('claude-sonnet-4-6-20250514'),
  OIDC_ISSUER: z.string().url().optional(),
  OIDC_AUDIENCE: z.string().optional(),
  MCP_URL: z.string().default('http://localhost:8080/mcp'),
  ABSTAIN_THRESHOLD: z.coerce.number().min(0).max(1).default(0.3),
  ESCALATION_THRESHOLD: z.coerce.number().min(0).max(1).default(0.5),
  MAX_ITERATIONS: z.coerce.number().int().min(1).default(5),
  MAX_TOKENS: z.coerce.number().int().min(100).default(4096),
});

export type Config = z.infer<typeof envSchema>;

export function loadConfig(): Config {
  return envSchema.parse(process.env);
}
