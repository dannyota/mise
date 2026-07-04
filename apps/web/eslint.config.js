import pluginVue from 'eslint-plugin-vue';
import tseslint from 'typescript-eslint';

export default [
  ...pluginVue.configs['flat/recommended'],
  ...tseslint.configs.recommended,
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
];
