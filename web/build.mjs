// Builds the frontend into dist/: bundles src/main.ts and src/preflight.ts and
// copies public/. The Docker build copies dist/ into internal/web/dist.
import { build } from 'esbuild';
import { cpSync, mkdirSync, rmSync } from 'node:fs';

rmSync('dist', { recursive: true, force: true });
mkdirSync('dist', { recursive: true });
cpSync('public', 'dist', { recursive: true });

const common = { bundle: true, minify: true, sourcemap: false, target: 'es2022', legalComments: 'none' };
await build({ ...common, entryPoints: ['src/main.ts'], outfile: 'dist/app.js', format: 'esm' });
// Preflight runs before first paint (theme, palette, font); a classic script, not a module.
await build({ ...common, entryPoints: ['src/preflight.ts'], outfile: 'dist/preflight.js', format: 'iife' });
console.log('web: built dist/');
