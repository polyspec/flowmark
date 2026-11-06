// Tests of the npm lock of the board example: npm ci fetches the tarball of a package from a URL only when the project names
// that URL among its own dependencies (allow-remote=root, examples/board/web/.npmrc), so every package that the lock resolves
// from a URL outside the registry stands in package.json with the same URL (T65-1).
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const BOARD = path.join(import.meta.dirname, '..', 'examples', 'board', 'web');
const REGISTRY = 'https://registry.npmjs.org/';

/** The record that `value` is, or an empty one. */
function record(value: unknown): Record<string, unknown> {
  return typeof value === 'object' && value !== null ? Object.fromEntries(Object.entries(value)) : {};
}

/** The packages of `lock` that resolve from a URL outside the registry, by name. */
export function remotePackages(lock: unknown): Map<string, string> {
  const remote = new Map<string, string>();
  for (const [key, entry] of Object.entries(record(record(lock).packages))) {
    const resolved = record(entry).resolved;
    if (key === '' || typeof resolved !== 'string' || resolved.startsWith(REGISTRY)) continue;
    remote.set(key.slice(key.lastIndexOf('node_modules/') + 'node_modules/'.length), resolved);
  }
  return remote;
}

/** The packages of `remote` that `manifest` does not name by the same URL among its dependencies. */
export function unnamedRemotes(manifest: unknown, remote: Map<string, string>): string[] {
  const named = { ...record(record(manifest).dependencies), ...record(record(manifest).devDependencies) };
  return [...remote].filter(([name, url]) => named[name] !== url).map(([name]) => name);
}

test('the board names every remote tarball of its lock among its dependencies', () => {
  const lock: unknown = JSON.parse(readFileSync(path.join(BOARD, 'package-lock.json'), 'utf8'));
  const manifest: unknown = JSON.parse(readFileSync(path.join(BOARD, 'package.json'), 'utf8'));
  const remote = remotePackages(lock);
  assert.ok(remote.size > 0, 'the board lock resolves the polyspec packages from their release tarballs');
  assert.deepEqual(unnamedRemotes(manifest, remote), []);
});

test('a remote tarball that only an override names is reported', () => {
  const url = 'https://github.com/polyspec/php-serialize/releases/download/v0.0.1/polyspec-php-serialize-0.0.1.tgz';
  const lock = { packages: { '': {}, 'node_modules/@polyspec/php-serialize': { resolved: url }, 'node_modules/a': { resolved: `${REGISTRY}a/-/a-1.0.0.tgz` } } };
  assert.deepEqual(unnamedRemotes({ dependencies: {}, overrides: { '@polyspec/php-serialize': url } }, remotePackages(lock)), ['@polyspec/php-serialize']);
  assert.deepEqual(unnamedRemotes({ dependencies: { '@polyspec/php-serialize': url } }, remotePackages(lock)), []);
  assert.deepEqual(unnamedRemotes({ dependencies: { '@polyspec/php-serialize': 'https://example.invalid/other.tgz' } }, remotePackages(lock)), ['@polyspec/php-serialize']);
});
