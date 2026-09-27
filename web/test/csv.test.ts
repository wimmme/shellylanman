import { test } from 'node:test';
import assert from 'node:assert/strict';
import { toCSV } from '../src/csv';

test('CSV with the configured separator, quoting only when needed', () => {
  assert.equal(toCSV(['Status', 'Name'], [['on line', 'Kitchen']], ','), 'Status,Name\r\non line,Kitchen\r\n');
  assert.equal(toCSV(['a'], [['x,y'], ['say "hi"'], ['two\nlines']], ','), 'a\r\n"x,y"\r\n"say ""hi"""\r\n"two\nlines"\r\n');
  assert.equal(toCSV(['a', 'b'], [['x,y', '1;2']], ';'), 'a;b\r\nx,y;"1;2"\r\n');
  assert.equal(toCSV(['a', 'b'], [['1', '2']], ''), 'a,b\r\n1,2\r\n');
});
