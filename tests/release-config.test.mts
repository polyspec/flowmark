// Each step of the workflows runs a target of the Makefile of the repository (T69).
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { matchesAny } from '@polyspec/kit/glob';
import { trackedFiles } from '@polyspec/kit/tracked-files';

const ROOT = path.join(import.meta.dirname, '..');

test('every run step of the workflows starts a target of the Makefile of the repository', () => {
  const makefile = readFileSync(path.join(ROOT, 'Makefile'), 'utf8');
  const targets = new Set([...makefile.matchAll(/^([a-z][a-z0-9 -]*):(?!=)/gm)].flatMap(match => (match[1] ?? '').split(' ')));
  for (const file of ['.github/workflows/ci.yml', '.github/workflows/documents.yml', '.github/workflows/release.yml', '.github/actions/bootstrap/action.yml']) {
    const text = readFileSync(path.join(ROOT, file), 'utf8');
    for (const [, target = ''] of text.matchAll(/^[ \t]*(?:run: )?make ([a-z][a-z0-9-]*)/gm)) {
      assert.ok(targets.has(target), `${file} runs make ${target}, which the Makefile does not declare`);
    }
  }
});

// The selection of ci.yml by the changed paths (T90): GitHub starts a push or pull_request run unless every changed path
// matches a pattern of paths-ignore, whose globs read `*` within one path segment and `**` across segments, as kit's glob.

/** The paths-ignore list of the trigger `event` of ci.yml, one quoted pattern per line; an event without it lists none. */
function pathsIgnore(event: string): string[] {
  const text = readFileSync(path.join(ROOT, '.github/workflows/ci.yml'), 'utf8');
  const trigger = text.split(/^on:\n/m)[1]?.split(/^\S/m)[0] ?? '';
  const block = trigger.split(new RegExp(`^  ${event}:\\n`, 'm'))[1]?.split(/^  \S/m)[0] ?? '';
  const list = block.split(/^    paths-ignore:\n/m)[1]?.split(/^    \S/m)[0] ?? '';
  return [...list.matchAll(/^      - '([^']+)'$/gm)].map(match => match[1] ?? '');
}

/** Whether the changed `paths` start the ci.yml run of `event`: some path matches no pattern of its paths-ignore. */
function ciStarts(event: string, paths: readonly string[]): boolean {
  const ignore = pathsIgnore(event);
  return paths.some(file => !matchesAny(ignore, file));
}

/** The JSON value of the repository file `file`. */
const json = (file: string): unknown => JSON.parse(readFileSync(path.join(ROOT, file), 'utf8'));
const isStrings = (v: unknown): v is string[] => Array.isArray(v) && v.every(x => typeof x === 'string');
/** The value of `key` of the object `v`, or undefined. */
const member = (v: unknown, key: string): unknown => (typeof v === 'object' && v !== null ? Object.entries(v).find(([name]) => name === key)?.[1] : undefined);
/** The strings of `key` of the object `v`; any other shape fails the test. */
function strings(v: unknown, key: string): string[] {
  const value = member(v, key);
  assert.ok(isStrings(value), `${key} is a list of strings`);
  return value;
}

const CHANGES: { name: string; paths: string[]; starts: boolean }[] = [
  { name: 'a document-only change, the checklist cells of a row', paths: ['docs/plans/flowmark/checklist.md', 'docs/plans/flowmark/checklist.ko.md'], starts: false },
  { name: 'a change of the root documents', paths: ['README.md', 'README.ko.md', 'AGENTS.md', 'AGENTS.ko.md', 'PRINCIPLES.md', 'CHANGELOG.md', 'CHANGELOG.ko.md'], starts: false },
  { name: 'a code change', paths: ['packages/go/store/corpus.go'], starts: true },
  { name: 'a code change pushed with documents', paths: ['docs/plans/flowmark/checklist.md', 'packages/go/store/corpus.go'], starts: true },
  { name: 'a spec, which the spec anchor test of test-go reads', paths: ['docs/spec/30-schema.md'], starts: true },
  { name: 'a Markdown fixture of a Go test', paths: ['packages/go/store/testdata/files/memos/M1.md'], starts: true },
  { name: 'a rebuild record, which rebuild-check reads', paths: ['docs/rebuild/alint.md'], starts: true },
  { name: 'an undeclared path', paths: ['notes.txt'], starts: true },
  { name: 'an undeclared Markdown path below the root', paths: ['examples/README.md'], starts: true },
  { name: 'the version commit of 0.0.13', paths: ['CHANGELOG.ko.md', 'CHANGELOG.md', 'docs/plans/flowmark/checklist.ko.md', 'docs/plans/flowmark/checklist.md', 'package-lock.json', 'package.json'], starts: true },
];

for (const event of ['push', 'pull_request']) {
  for (const change of CHANGES) {
    test(`ci.yml on ${event}: ${change.name} ${change.starts ? 'starts every job' : 'starts no run'}`, () => {
      assert.equal(ciStarts(event, change.paths), change.starts, `${change.paths.join(', ')} against paths-ignore ${JSON.stringify(pathsIgnore(event))}`);
    });
  }
}

test('ci.yml ignores no package file of config/release.json, so a version commit runs every job', () => {
  const release = json('config/release.json');
  const manifests = member(release, 'manifests');
  assert.ok(typeof manifests === 'object' && manifests !== null, 'config/release.json has manifests');
  for (const event of ['push', 'pull_request']) {
    for (const file of [...strings(release, 'packageFiles'), ...Object.keys(manifests)]) assert.ok(ciStarts(event, [file]), `ci.yml on ${event} ignores ${file}`);
  }
});

test('every tracked path that ci.yml ignores is read only by the checks of the documents workflow', () => {
  const makefile = readFileSync(path.join(ROOT, 'Makefile'), 'utf8');
  const documentChecks = new Set((/^DOCUMENT_CHECKS := (.+)$/m.exec(makefile)?.[1] ?? '').split(' ').filter(Boolean));
  assert.ok(documentChecks.size > 0, 'the Makefile declares DOCUMENT_CHECKS');
  const owners = member(json('config/owner-checks.json'), 'owners');
  assert.ok(Array.isArray(owners), 'config/owner-checks.json has owners');
  const rules = owners.map((rule: unknown) => ({ paths: strings(rule, 'paths'), targets: strings(rule, 'targets') }));
  const ignore = pathsIgnore('push');
  for (const file of trackedFiles(ROOT).filter(file => matchesAny(ignore, file))) {
    for (const owner of rules.filter(rule => matchesAny(rule.paths, file))) {
      for (const target of owner.targets) assert.ok(documentChecks.has(target), `${file} is ignored by ci.yml, but ${target} reads it`);
    }
  }
});

test('the documents workflow runs on every push and pull request that ci.yml filters, with no path filter, and checks the size of the diff', () => {
  const text = readFileSync(path.join(ROOT, '.github/workflows/documents.yml'), 'utf8');
  assert.match(text, /^on:\n  push:\n    branches: \[main\]\n  pull_request:\n  workflow_dispatch:\n/m);
  assert.doesNotMatch(text, /paths(-ignore)?:/);
  assert.match(text, /^\s+run: make diff-size-check$/m);
  assert.match(text, /^\s+run: make documents$/m);
  assert.deepEqual(pathsIgnore('pull_request'), pathsIgnore('push'));
});
