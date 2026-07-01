import { z } from 'zod';

const envSchema = z.object({
  PORT: z.coerce.number().default(3001),
  SERVING_URL: z.string().default('http://localhost:8080'),
  NODE_ENV: z
    .enum(['development', 'production', 'test'])
    .default('development'),
});

export type Config = z.infer<typeof envSchema>;

/** loadConfig parses process.env once into a typed config, failing fast on bad input. */
export function loadConfig(): Config {
  return envSchema.parse(process.env);
}
