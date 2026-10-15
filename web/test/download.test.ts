import { test } from 'node:test';
import assert from 'node:assert/strict';
import { localPath, megabytes, progressText } from '../src/download';

test('the download button uses the link relative to the page (own port, proxy, ingress)', () => {
  assert.equal(localPath('http://192.168.0.127:3082/fw/TOKEN/MiniPMG3-2.1.0.zip'), 'fw/TOKEN/MiniPMG3-2.1.0.zip');
  assert.equal(localPath('https://shellylanman.example/fw/T/a.zip'), 'fw/T/a.zip');
  assert.equal(localPath('http://x/other'), null);
});

test('sizes and progress text', () => {
  assert.equal(megabytes(0), '0.1 MB');
  assert.equal(megabytes(1572864), '1.5 MB');
  assert.equal(progressText(1048576, 3145728), '1.0 MB / 3.0 MB');
  assert.equal(progressText(1048576, 0), '1.0 MB');
});
