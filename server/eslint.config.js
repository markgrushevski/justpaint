import pluginJs from '@eslint/js';
import eslintConfigPrettier from 'eslint-config-prettier';
import globals from 'globals';

export default /** @type { import('eslint').Linter.FlatConfig[] } */ ([
    {
        files: ['src/**/*.js'],
        languageOptions: {
            globals: globals.nodeBuiltin,
            parserOptions: { ecmaVersion: 'latest', sourceType: 'module' }
        }
    },
    pluginJs.configs.recommended,
    eslintConfigPrettier
]);
