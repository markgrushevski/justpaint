import globals from 'globals'
import eslint from '@eslint/js'
import pluginTs from 'typescript-eslint'
import pluginVue from 'eslint-plugin-vue'
import pluginPrettier from 'eslint-config-prettier'

/** @type {import('@typescript-eslint/utils').TSESLint.FlatConfig.ConfigFile} */
export default [
    { ignores: ['.idea', '.vscode', 'dist', 'node_modules', 'types', 'cache'] },
    eslint.configs.recommended,
    ...pluginTs.configs.recommended,
    ...pluginVue.configs['flat/recommended'],
    {
        // The app is browser code — provide browser globals to all source.
        // eslint-plugin-vue v10 no longer injects these via its shared config,
        // so they must be declared here (was mis-scoped to a nonexistent lib/).
        files: ['**/*.{js,ts,vue}'],
        languageOptions: {
            globals: { ...globals.browser },
            parserOptions: { ecmaVersion: 'latest' }
        }
    },
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
