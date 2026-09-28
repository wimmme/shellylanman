// Builds the frontend into dist/: bundles src/main.ts and src/preflight.ts and
// copies public/. The Docker build copies dist/ into internal/web/dist.
import { build } from 'esbuild';
import { cpSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';

rmSync('dist', { recursive: true, force: true });
mkdirSync('dist', { recursive: true });
cpSync('public', 'dist', { recursive: true });

// The About page lists the packages bundled into the frontend (not the build tools).
const lock = JSON.parse(readFileSync('package-lock.json', 'utf8'));
const deps = Object.entries(lock.packages)
  .filter(([path, p]) => path.startsWith('node_modules/') && !p.dev && !path.includes('@types/'))
  .map(([path, p]) => ({ name: path.slice('node_modules/'.length), version: p.version, license: p.license ?? '' }))
  .sort((a, b) => a.name.localeCompare(b.name));
writeFileSync('dist/deps.json', JSON.stringify(deps));

const common = { bundle: true, minify: true, sourcemap: false, target: 'es2022', legalComments: 'none' };
// The script editor (CodeMirror) is imported dynamically: splitting puts it in its own chunk.
await build({ ...common, entryPoints: { app: 'src/main.ts' }, outdir: 'dist', format: 'esm', splitting: true, chunkNames: 'chunks/[name]-[hash]' });
// Preflight runs before first paint (theme, palette, font); a classic script, not a module.
await build({ ...common, entryPoints: ['src/preflight.ts'], outfile: 'dist/preflight.js', format: 'iife' });
console.log('web: built dist/');
