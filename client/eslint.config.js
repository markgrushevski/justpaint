import globals from 'globals'
import eslint from '@eslint/js'
import pluginTs from 'typescript-eslint'
import pluginVue from 'eslint-plugin-vue'
import pluginPrettier from 'eslint-config-prettier'

/** @type {import('@typescript-eslint/utils').TSESLint.FlatConfig.ConfigFile} */
export default [
    {
        files: ['lib/**/*.{ts,vue}'],
        languageOptions: {
            globals: globals.browser,
            parserOptions: { ecmaVersion: 'latest' }
        },
        ignores: ['.idea', '.vscode', 'dist', 'node_modules', 'types', 'cache']
    },
    eslint.configs.recommended,
    ...pluginTs.configs.recommended,
    ...pluginVue.configs['flat/recommended'],
    {
        files: ['**/*.vue'],
        languageOptions: {
            parserOptions: {
                parser: '@typescript-eslint/parser'
            }
        },
        rules: {
            'vue/require-default-prop': 'off'
        }
    },
    pluginPrettier
]
