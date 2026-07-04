import pluginVue from 'eslint-plugin-vue';
import tseslint from 'typescript-eslint';

export default tseslint.config(
  ...tseslint.configs.recommended,
  ...pluginVue.configs['flat/recommended'],
  {
    files: ['**/*.vue'],
    languageOptions: {
      parserOptions: {
        parser: tseslint.parser,
      },
    },
  },
  {
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            '@anthropic-ai/*',
            '@google-cloud/aiplatform',
            '@google-cloud/vertexai',
            'openai',
            'claude-agent-sdk',
            '@anthropic-ai/sdk',
          ],
        },
      ],
    },
  },
);
