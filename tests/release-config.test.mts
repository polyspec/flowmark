// Tests of the workflow that the release takes: make release-tag of kit releases the commit of a successful push run of
// .github/workflows/ci.yml on main, and that run succeeds only through its job ci-passed, so ci-passed needs every other job of
// ci.yml, the gates of kit among them.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { parse } from 'yaml';

const ROOT = path.join(import.meta.dirname, '..');

/** The record that `value` is, or an empty one. */
function record(value: unknown): Record<string, unknown> {
  return typeof value === 'object' && value !== null ? Object.fromEntries(Object.entries(value)) : {};
}

test('ci-passed of ci.yml needs every other job of ci.yml, the gates of kit among them', () => {
  const workflow: unknown = parse(readFileSync(path.join(ROOT, '.github/workflows/ci.yml'), 'utf8'));
  const jobs = record(record(workflow).jobs);
  assert.equal(record(jobs.kit).uses, './.github/workflows/ci-pipeline.yml');
  const { needs }: { needs?: unknown } = record(jobs['ci-passed']);
  assert.ok(Array.isArray(needs) && needs.every(job => typeof job === 'string'));
  const byName = (left: string, right: string) => left.localeCompare(right);
  assert.deepEqual(needs.toSorted(byName), Object.keys(jobs).filter(id => id !== 'ci-passed').toSorted(byName));
});
