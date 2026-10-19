// Tests of the case-sensitive folder on macOS (scripts/go-test.mts): a volume already mounted at the folder is reused and not
// detached. Only the macOS run of make flowmark-test reads this folder, since hdiutil exists only there.
import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { spawnSync } from 'node:child_process';
import { provision } from '../../scripts/go-test.mts';

// A mount found already at the folder is reused and left mounted: the run that
// attached it, such as a concurrent make flowmark-test, still needs it.
test('a volume already mounted at the folder is reused, not detached', t => {
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

