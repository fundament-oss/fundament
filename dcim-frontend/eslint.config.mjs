// @ts-check
import tseslint from 'typescript-eslint';
import angular from 'angular-eslint';
import { configs as airbnbConfigs, plugins as airbnbPlugins } from 'eslint-config-airbnb-extended';
import eslintConfigPrettier from 'eslint-config-prettier';

export default tseslint.config(
  {
    ignores: ['src/generated/**'],
  },
  tseslint.configs.recommended,
  airbnbPlugins.stylistic,
  airbnbPlugins.importX,
  {
    files: ['**/*.ts'],
    extends: [
      ...airbnbConfigs.base.all,
      ...tseslint.configs.recommended,
      ...tseslint.configs.stylistic,
      ...angular.configs.tsRecommended,
      eslintConfigPrettier,
    ],
    processor: angular.processInlineTemplates,
    rules: {
      '@angular-eslint/directive-selector': [
        'error',
        {
          type: 'attribute',
          prefix: 'app',
          style: 'camelCase',
        },
      ],
      '@angular-eslint/component-selector': [
        'error',
        {
          type: 'element',
          prefix: 'app',
          style: 'kebab-case',
        },
      ],
      '@typescript-eslint/class-methods-use-this': [
        'error',
        {
          enforceForClassFields: false,
          exceptMethods: [
            'ngOnInit',
            'ngOnChanges',
            'ngDoCheck',
            'ngAfterContentInit',
            'ngAfterContentChecked',
            'ngAfterViewInit',
            'ngAfterViewChecked',
            'ngOnDestroy',
          ],
        },
      ],
      'class-methods-use-this': [
        'error',
        {
          enforceForClassFields: false,
          exceptMethods: [
            'ngOnInit',
            'ngOnChanges',
            'ngDoCheck',
            'ngAfterContentInit',
            'ngAfterContentChecked',
            'ngAfterViewInit',
            'ngAfterViewChecked',
            'ngOnDestroy',
          ],
        },
      ],
    },
  },
  {
    files: ['src/plugin-sdk/build-css.ts'],
    rules: {
      'import-x/no-extraneous-dependencies': ['error', { devDependencies: true }],
      'no-console': 'off',
    },
  },
  {
    files: ['**/*.html'],
    extends: [...angular.configs.templateRecommended, ...angular.configs.templateAccessibility],
    rules: {},
  },
);
