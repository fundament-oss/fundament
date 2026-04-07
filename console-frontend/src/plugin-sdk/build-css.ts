/**
 * Builds plugin-sdk.css using PostCSS + Tailwind v4.
 * Run via: bun src/plugin-sdk/build-css.ts
 */
import postcss from 'postcss';
import tailwindcss from '@tailwindcss/postcss';
import { readFileSync, writeFileSync } from 'fs';
import { resolve } from 'path';

const root = resolve(import.meta.dir, '../..');
const input = resolve(root, 'src/plugin-sdk/plugin-sdk.css');
const output = resolve(root, 'public/plugin-ui/plugin-sdk.css');

const css = readFileSync(input, 'utf8');

const result = await postcss([tailwindcss]).process(css, {
  from: input,
  to: output,
});

writeFileSync(output, result.css);
console.log('Built plugin-sdk.css');
