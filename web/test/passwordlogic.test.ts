import { test } from 'node:test';
import assert from 'node:assert/strict';
import { problems, strength } from '../src/passwordlogic';

test('rules: 8 characters and a capital, as the server', () => {
  assert.deepEqual(problems(''), ['tooShort', 'noCapital']);
  assert.deepEqual(problems('Abcdefg'), ['tooShort']);
  assert.deepEqual(problems('abcdefgh'), ['noCapital']);
  assert.deepEqual(problems('Abcdefgh'), []);
  assert.deepEqual(problems('Ébène-déjà'), []);
  assert.deepEqual(problems('短密码短密码短密A'), []);
});

test('strength grows with length and kinds of characters', () => {
  assert.equal(strength(''), 0);
  assert.equal(strength('abc'), 0);
  assert.equal(strength('abcdefgh'), 1); // too weak for the rules
  assert.equal(strength('Abcdefgh'), 1);
  assert.equal(strength('Kitchen7lamp'), 3);
  assert.equal(strength('Kitchen-7-lamp!'), 4);
  assert.equal(strength('Bluebird harbour lantern'), 4); // a long passphrase
});

test('common words and repeats stay weak', () => {
  assert.equal(strength('Password123!'), 1);
  assert.equal(strength('MyShelly2026!'), 1);
  assert.equal(strength('Aaaaaaaaaaaa1!'), 1);
});
