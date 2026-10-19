// Tests of the Go test runner (scripts/go-test.mts): a run whose PATH holds no `go` fails naming the cause and the fix,
// not with the bare spawn error.
import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { spawnSync } from 'node:child_process';
import { provision, runGoTests } from '../scripts/go-test.mts';

test('a PATH that holds no go fails naming go, the PATH searched and the fix', t => {
  const root = mkdtempSync(path.join(tmpdir(), 'go-test-'));
  const empty = mkdtempSync(path.join(tmpdir(), 'go-test-path-'));
  const saved = process.env.PATH;
  t.after(() => {
    process.env.PATH = saved;
    rmSync(root, { recursive: true, force: true });
    rmSync(empty, { recursive: true, force: true });
  });
  process.env.PATH = empty;
  assert.throws(() => runGoTests(root, root), error => {
    assert.match(error.message, /no executable named go on PATH/);
    assert.ok(error.message.includes(empty), `the message names the PATH searched: ${error.message}`);
    assert.match(error.message, /install Go or put its bin folder on PATH/);
    return true;
  });
});
