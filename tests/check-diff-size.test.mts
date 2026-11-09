// Tests of the size check of a diff (scripts/check-diff-size.mts, T90): the diff of each event is the one that GitHub
// compares, a diff of more than 3000 files fails, and the count is the files of a Git checkout.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { changedFiles, diffOf, judge, LIMIT } from '../scripts/check-diff-size.mts';

const BEFORE = 'a'.repeat(40);
const AFTER = 'b'.repeat(40);

test('a push of an existing branch compares its before and after commits by a two-dot diff', () => {
  assert.deepEqual(diffOf('push', { ref: 'refs/heads/main', before: BEFORE, after: AFTER }), { range: `${BEFORE}..${AFTER}` });
});

test('a pull request compares its base and head by a three-dot diff', () => {
  assert.deepEqual(diffOf('pull_request', { pull_request: { base: { sha: BEFORE }, head: { sha: AFTER } } }), { range: `${BEFORE}...${AFTER}` });
});

test('a dispatch has no path filter and nothing to count', () => {
  assert.ok('none' in diffOf('workflow_dispatch', {}));
});

test('a push that creates its branch fails and names the branch, since it has no before commit', () => {
  assert.throws(() => diffOf('push', { ref: 'refs/heads/main', before: '0'.repeat(40), after: AFTER }), /creates refs\/heads\/main/);
});

test('an event without its commits fails and names the missing field', () => {
  assert.throws(() => diffOf('pull_request', { pull_request: { base: { sha: BEFORE } } }), /pull_request\.head\.sha/);
  assert.throws(() => diffOf('push', { before: BEFORE }), /after/);
});

test('an event that ci.yml does not filter by its paths fails', () => {
  assert.throws(() => diffOf('schedule', {}), /schedule/);
});

test(`a diff of ${LIMIT} files passes and one of ${LIMIT + 1} fails and names the count`, () => {
  assert.equal(judge('x..y', LIMIT), undefined);
  assert.match(judge('x..y', LIMIT + 1) ?? '', /x\.\.y changes 3001 files, more than the 3000/);
});

test('the count is the changed files of the range, a rename as its two paths', () => {
  const root = mkdtempSync(path.join(tmpdir(), 'check-diff-size-'));
  try {
    const git = (...args: string[]) => execFileSync('git', ['-c', 'user.name=t', '-c', 'user.email=t@t', ...args], { cwd: root, encoding: 'utf8' }).trim();
    git('init', '-q');
    writeFileSync(path.join(root, 'a.md'), 'a\n');
    writeFileSync(path.join(root, 'b.go'), 'package b\n');
    git('add', '.');
    git('commit', '-q', '-m', 'one');
    const before = git('rev-parse', 'HEAD');
    writeFileSync(path.join(root, 'a.md'), 'a2\n');
    git('mv', 'b.go', 'c.go');
    writeFileSync(path.join(root, 'with space.txt'), 'x\n');
    git('add', '.');
    git('commit', '-q', '-m', 'two');
    assert.equal(changedFiles(root, `${before}..HEAD`), 4);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
