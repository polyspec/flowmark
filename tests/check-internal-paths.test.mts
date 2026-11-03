// Tests of the internal-path check (scripts/check-internal-paths.mts): a line that names the source or the build output of a
// dependency is a finding, and a declared bin or a text without such a path is not. The paths are joined here, so that the check,
// which reads this file too, does not see them.
import assert from 'node:assert/strict';
import test from 'node:test';
import { scanText } from '../scripts/check-internal-paths.mts';

const named = (...parts: string[]): string => parts.join('/');

const REPORTED: [string, string][] = [
  ['a scoped package build output', named('node_modules', '@polyspec', 'orm', 'dist', 'bin', 'orm-gen.js')],
  ['a plain package source', named('node_modules', 'yaml', 'src', 'index.ts')],
  ['a composer package source', named('vendor', 'polyspec', 'orm', 'src', 'Db.php')],
  ['a composer package build output', named('vendor', 'polyspec', 'orm', 'dist', 'orm.phar')],
];
for (const [name, line] of REPORTED) {
  test(`scanText reports ${name}`, () => {
    assert.deepEqual(scanText('a.md', `first\n${line}\n`).map(finding => [finding.file, finding.line, finding.rule]), [['a.md', 2, 'internal-path']]);
  });
}

const ACCEPTED: [string, string][] = [
  ['a declared bin', named('node_modules', '.bin', 'orm-gen')],
  ['a package root', named('node_modules', '@polyspec', 'orm', 'package.json')],
  ['a source folder of this repository', named('packages', 'go', 'src', 'a.go')],
  ['an empty text', ''],
];
for (const [name, line] of ACCEPTED) {
  test(`scanText accepts ${name}`, () => {
    assert.deepEqual(scanText('a.md', line), []);
  });
}
