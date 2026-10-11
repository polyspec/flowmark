// The Go tests of the repository, with the case-sensitive folder they need. A test that writes two names differing only
// in case (validate's TestFilesCaseDuplicate) reads the folder from FLOWMARK_CASE_SENSITIVE_DIR when its temporary
// folder folds case; this script declares that folder at var/case-sensitive under the repository root and provisions it
// by platform: on darwin a case-sensitive APFS image mounted there (a mount already at that one place is reused), on
// win32 a folder whose per-directory case sensitivity fsutil enables, elsewhere a plain folder, whose file system keeps
// case. The folder is then probed, and a folder that folds case fails the run, naming the step that should have made it
// case-sensitive. The exit status is go test's.
import { existsSync, mkdirSync, rmSync, statSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath, pathToFileURL } from 'node:url';

/** The repository root: the parent of the folder that holds this script. */
export const repositoryRoot = path.dirname(path.dirname(fileURLToPath(import.meta.url)));

/** The environment variable that names the case-sensitive folder to the Go tests. */
export const caseSensitiveEnv = 'FLOWMARK_CASE_SENSITIVE_DIR';

const log = message => console.log(`[go-test] ${message}`);

/** Runs `command` with `args`, its output shown; throws when it does not exit 0. */
function run(command, args, options = {}) {
  const result = spawnSync(command, args, { stdio: 'inherit', ...options });
  if (result.error) throw new Error(`${command} ${args.join(' ')}: ${result.error.message}`);
  if (result.status !== 0) throw new Error(`${command} ${args.join(' ')} exited with ${result.status ?? result.signal}`);
}

/** Whether `dir` is a mount point: its device differs from its parent's. */
function isMountPoint(dir) {
  return existsSync(dir) && statSync(dir).dev !== statSync(path.dirname(dir)).dev;
}

/** Whether the file system of `dir` folds case: a file written as CaseProbe is found as caseprobe. */
export function foldsCase(dir) {
  const probe = path.join(dir, 'CaseProbe');
  writeFileSync(probe, '');
  try {
    return existsSync(path.join(dir, 'caseprobe'));
  } finally {
    rmSync(probe);
  }
}

/**
 * Provisions the case-sensitive folder `dir` on `platform` and returns the name of the step that made it so, and
 * whether this run attached an image there that it must detach.
 */
function provision(dir, platform) {
  mkdirSync(path.dirname(dir), { recursive: true });
  switch (platform) {
    case 'darwin': {
      if (isMountPoint(dir)) return { step: `the volume already mounted at ${dir}`, attached: true };
      const image = `${dir}.dmg`;
      run('hdiutil', ['create', '-size', '64m', '-fs', 'Case-sensitive APFS', '-volname', 'flowmark-case', '-ov', image]);
      mkdirSync(dir, { recursive: true });
      run('hdiutil', ['attach', '-nobrowse', '-mountpoint', dir, image]);
      return { step: `the Case-sensitive APFS image ${image} attached at ${dir}`, attached: true };
    }
    case 'win32':
      mkdirSync(dir, { recursive: true });
      run('fsutil.exe', ['file', 'setCaseSensitiveInfo', dir, 'enable']);
      return { step: `fsutil.exe file setCaseSensitiveInfo ${dir} enable`, attached: false };
    default:
      mkdirSync(dir, { recursive: true });
      return { step: `the folder ${dir} on the ${platform} file system`, attached: false };
  }
}

/** Runs the Go tests with the case-sensitive folder and returns go test's exit status. */
export function goTest(root = repositoryRoot, platform = process.platform) {
  const dir = path.join(root, 'var', 'case-sensitive');
  let attached = false;
  try {
    const provisioned = provision(dir, platform);
    attached = provisioned.attached;
    if (foldsCase(dir)) {
      log(`${dir} folds case on ${platform}: ${provisioned.step} did not make it case-sensitive`);
      return 1;
    }
    log(`${caseSensitiveEnv}=${dir} (${provisioned.step})`);
    return runGoTests(root, dir);
  } finally {
    if (platform === 'darwin' && attached) run('hdiutil', ['detach', dir]);
  }
}

/**
 * Runs `go test` over the module at `root` with the case-sensitive folder `dir` declared, and returns its exit status.
 * The environment is the process's, extended with that one variable, so `go` is looked up on the process's PATH at the
 * moment of the spawn: a PATH without it — Go not installed, or its link absent while a package manager relinks it —
 * fails naming the PATH searched and the fix, never the bare spawn error.
 */
export function runGoTests(root, dir) {
  const env = { ...process.env, [caseSensitiveEnv]: dir };
  const result = spawnSync('go', ['test', '-count=1', './...'], { cwd: root, stdio: 'inherit', env });
  if (result.error?.code === 'ENOENT') {
    throw new Error(
      `go test: no executable named go on PATH (${env.PATH ?? ''}); install Go or put its bin folder on PATH, then run again`,
    );
  }
  if (result.error) throw new Error(`go test: ${result.error.message}`);
  return result.status ?? 1;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    process.exitCode = goTest();
  } catch (error) {
    log(error.message);
    process.exitCode = 1;
  }
}
