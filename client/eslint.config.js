import pluginJs from '@eslint/js'
import eslintConfigPrettier from 'eslint-config-prettier'
import pluginVue from 'eslint-plugin-vue'
import globals from 'globals'
import tsESLint from 'typescript-eslint'

export default /** @type { import('eslint').Linter.FlatConfig[] } */ ([
    pluginJs.configs.recommended,
    ...tsESLint.configs.recommended,
    ...pluginVue.configs['flat/recommended'],
    eslintConfigPrettier,
    {
        files: ['src/**/*.{ts,vue}'],
        languageOptions: {
            globals: globals.browser,
            parserOptions: { ecmaVersion: 'latest', sourceType: 'module' }
        }
    }
])
