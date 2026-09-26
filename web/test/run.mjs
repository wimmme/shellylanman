// Runs every test/*.test.ts: bundles each with esbuild, then `node --test`.
// A new test file is picked up without registering it anywhere.
import { build } from 'esbuild';
import { readdirSync, mkdirSync, rmSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { join } from 'node:path';

const out = join('test', '.out');
rmSync(out, { recursive: true, force: true });
mkdirSync(out, { recursive: true });

const tests = readdirSync('test').filter((f) => f.endsWith('.test.ts'));
if (tests.length === 0) {
  console.error('no tests found');
  process.exit(1);
}
const outputs = [];
for (const t of tests) {
  const outfile = join(out, t.replace(/\.ts$/, '.mjs'));
  await build({ entryPoints: [join('test', t)], outfile, bundle: true, format: 'esm', platform: 'node', target: 'node22' });
  outputs.push(outfile);
}
const r = spawnSync(process.execPath, ['--test', ...outputs], { stdio: 'inherit' });
process.exit(r.status ?? 1);
