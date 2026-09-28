import { test } from 'node:test';
import assert from 'node:assert/strict';
import { parseChangelog, plain } from '../src/changelog';

const MD = `# Changelog

Intro text.

## [Unreleased]

## [0.2.0] - 2026-10-01

Short summary
over two lines.

### Added
- **Bold** thing with \`code\` and a [link](https://x).
- A long item
  that continues.

### Fixed
- One fix.

## [0.1.0] - 2026-09-28

### Added — Phase 10: release
- First.
`;

test('releases, groups and items are read; empty Unreleased is dropped', () => {
  const r = parseChangelog(MD);
  assert.deepEqual(r.map((x) => x.version), ['0.2.0', '0.1.0']);
  assert.equal(r[0]!.date, '2026-10-01');
  assert.equal(r[0]!.summary, 'Short summary over two lines.');
  assert.deepEqual(r[0]!.groups.map((g) => g.title), ['Added', 'Fixed']);
  assert.deepEqual(r[0]!.groups[0]!.items, ['Bold thing with code and a link.', 'A long item that continues.']);
  assert.equal(r[1]!.groups[0]!.title, 'Added — Phase 10: release');
});

test('plain drops markdown decoration', () => {
  assert.equal(plain('**a** `b` [c](d)'), 'a b c');
});
