// Tests of the workflow that the release takes: make release-tag of kit releases the commit of a successful push run of
// .github/workflows/ci.yml on main, and that run succeeds only when every job of it succeeds (the conclusion of the run that
// release-run.mjs reads), so every other job needs kit, the gates of kit, and no job repeats that conclusion (T15-51, S12).
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

test('ci.yml runs the gates of kit first, and every other job of it needs kit', () => {
  const workflow: unknown = parse(readFileSync(path.join(ROOT, '.github/workflows/ci.yml'), 'utf8'));
  const jobs = record(record(workflow).jobs);
  assert.equal(record(jobs.kit).uses, './.github/workflows/ci-pipeline.yml');
  for (const [id, job] of Object.entries(jobs)) {
    if (id === 'kit') continue;
    assert.deepEqual(record(job).needs, 'kit', `job ${id} of ci.yml needs kit`);
  }
});

test('ci.yml has no job that repeats the conclusion of the run', () => {
  const workflow: unknown = parse(readFileSync(path.join(ROOT, '.github/workflows/ci.yml'), 'utf8'));
  const jobs = record(record(workflow).jobs);
  assert.equal(jobs['ci-passed'], undefined, 'ci-passed is deleted: the run already fails when one of its jobs fails');
});
