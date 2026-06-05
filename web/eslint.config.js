import js from '@eslint/js'
import tseslint from 'typescript-eslint'
import pluginVue from 'eslint-plugin-vue'
import prettierConfig from 'eslint-config-prettier'
import prettierPlugin from 'eslint-plugin-prettier'
import globals from 'globals'

// Relaxed rules for gradual adoption — tighten as codebase matures
const gradualRules = {
  // Warn on prop mutation instead of silently allowing it
  'vue/no-mutating-props': 'warn',
  // TypeScript compiler covers undef checks; keep off to avoid SFC template false positives
  'no-undef': 'off',
}

export default tseslint.config(
  {
    ignores: ['dist/**', 'node_modules/**', '.quasar/**', 'src/services/**'],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  ...pluginVue.configs['flat/recommended'],
  prettierConfig,
  {
    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.node,
      },
    },
    rules: gradualRules,
  },
  {
    files: ['**/*.vue'],
    languageOptions: {
      parserOptions: {
        parser: tseslint.parser,
      },
      globals: {
        ...globals.browser,
        $q: 'readonly',
        Quasar: 'readonly',
      },
    },
    rules: gradualRules,
  },
  {
    plugins: {
      prettier: prettierPlugin,
    },
    rules: {
      'prettier/prettier': 'warn',
      'vue/multi-word-component-names': 'off',
      '@typescript-eslint/no-explicit-any': 'warn',
      '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_' }],
    },
  },
)
