// Tests of the Go test runner (scripts/go-test.mjs): a run whose PATH holds no `go` fails naming the cause and the fix,
// not with the bare spawn error.
import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { spawnSync } from 'node:child_process';
import { provision, runGoTests } from '../scripts/go-test.mjs';

// A mount found already at the folder is reused and left mounted: the run that
// attached it, such as a concurrent make flowmark-test, still needs it.
test('a volume already mounted at the folder is reused, not detached', { skip: process.platform !== 'darwin' }, t => {
  const root = mkdtempSync(path.join(tmpdir(), 'go-test-mount-'));
  const dir = path.join(root, 'case-sensitive');
  const image = path.join(root, 'probe.dmg');
  const run = (...args) => assert.equal(spawnSync('hdiutil', args, { stdio: 'ignore' }).status, 0, `hdiutil ${args.join(' ')}`);
  run('create', '-size', '8m', '-fs', 'Case-sensitive APFS', '-volname', 'flowmark-probe', '-ov', image);
  mkdirSync(dir);
  run('attach', '-nobrowse', '-mountpoint', dir, image);
  t.after(() => {
    spawnSync('hdiutil', ['detach', dir], { stdio: 'ignore' });
    rmSync(root, { recursive: true, force: true });
  });
  const provisioned = provision(dir, 'darwin');
  assert.equal(provisioned.attached, false, `a reused mount is reported attached: ${provisioned.step}`);
});

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
