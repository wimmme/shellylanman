import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';

// Installable as a PWA (DECISIONS P12-10): a manifest with 192 and 512 px icons,
// relative URLs (the app also runs under Home Assistant's ingress path).
test('the web app manifest is complete and linked', () => {
  const m = JSON.parse(readFileSync('public/manifest.json', 'utf8')) as {
    name: string; start_url: string; scope: string; display: string; icons: { src: string; sizes: string; purpose: string }[];
  };
  assert.equal(m.name, 'ShellyLanMan');
  assert.equal(m.display, 'standalone');
  assert.equal(m.start_url, './');
  assert.equal(m.scope, './');
  for (const size of ['192x192', '512x512']) assert.ok(m.icons.some((i) => i.sizes === size && i.purpose === 'any'), size);
  assert.ok(m.icons.some((i) => i.purpose === 'maskable'));
  for (const i of m.icons) {
    assert.ok(!i.src.startsWith('/'), `${i.src} must be relative`);
    const png = readFileSync('public/' + i.src);
    const [w, hgt] = [png.readUInt32BE(16), png.readUInt32BE(20)];
    assert.equal(`${w}x${hgt}`, i.sizes, i.src);
  }
  assert.ok(existsSync('public/apple-touch-icon.png'));
  assert.match(readFileSync('public/index.html', 'utf8'), /<link rel="manifest" href="manifest\.json">/);
});
