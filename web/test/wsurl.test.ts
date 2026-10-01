import { test } from 'node:test';
import assert from 'node:assert/strict';
import { wsURL } from '../src/api';

test('WebSocket URLs are relative to the page (Home Assistant ingress prefix)', () => {
  const g = globalThis as unknown as { location?: { href: string } };
  const saved = g.location;
  try {
    g.location = { href: 'https://ha.example/api/hassio_ingress/abc123/#/devices' };
    assert.equal(wsURL('ws'), 'wss://ha.example/api/hassio_ingress/abc123/ws');
    assert.equal(wsURL('ws/log/AABB'), 'wss://ha.example/api/hassio_ingress/abc123/ws/log/AABB');
    g.location = { href: 'http://192.168.0.12:3082/#/settings' };
    assert.equal(wsURL('ws'), 'ws://192.168.0.12:3082/ws');
  } finally {
    g.location = saved;
  }
});
