// Builds the frontend into dist/: bundles src/main.ts and src/preflight.ts and
// copies public/. The Docker build copies dist/ into internal/web/dist.
import { build } from 'esbuild';
import { cpSync, mkdirSync, rmSync } from 'node:fs';

rmSync('dist', { recursive: true, force: true });
mkdirSync('dist', { recursive: true });
cpSync('public', 'dist', { recursive: true });

const common = { bundle: true, minify: true, sourcemap: false, target: 'es2022', legalComments: 'none' };
// The script editor (CodeMirror) is imported dynamically: splitting puts it in its own chunk.
await build({ ...common, entryPoints: { app: 'src/main.ts' }, outdir: 'dist', format: 'esm', splitting: true, chunkNames: 'chunks/[name]-[hash]' });
// Preflight runs before first paint (theme, palette, font); a classic script, not a module.
await build({ ...common, entryPoints: ['src/preflight.ts'], outfile: 'dist/preflight.js', format: 'iife' });
console.log('web: built dist/');
