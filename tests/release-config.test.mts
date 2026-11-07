// Tests of the workflow that the release takes: make release-tag releases the commit of a successful push run of
// .github/workflows/ci.yml on main, and that run succeeds only when every job of it succeeds (the conclusion of the run that
// release-run.mjs of kit reads), so every other job needs the gates, and no job repeats that conclusion (T15-51). Each step of
// the workflow runs a target of the Makefile of the repository, which kit includes no longer (T69).
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

test('ci.yml runs its own gates first, and every other job of it needs them', () => {
  const workflow: unknown = parse(readFileSync(path.join(ROOT, '.github/workflows/ci.yml'), 'utf8'));
  const jobs = record(record(workflow).jobs);
  const gates = record(jobs.gates);
  assert.equal(gates.uses, undefined, 'the gates job runs the targets of the Makefile, not a workflow of kit');
  assert.ok(JSON.stringify(gates.steps).includes('make gates'), 'the gates job runs make gates');
  for (const [id, job] of Object.entries(jobs)) {
    if (id === 'gates') continue;
    assert.deepEqual(record(job).needs, 'gates', `job ${id} of ci.yml needs gates`);
  }
});

test('every run step of the workflows starts a target of the Makefile of the repository', () => {
  const makefile = readFileSync(path.join(ROOT, 'Makefile'), 'utf8');
  assert.equal(/^include /m.test(makefile), false, 'the Makefile includes no recipe of kit');
  const targets = new Set([...makefile.matchAll(/^([a-z][a-z0-9 -]*):(?!=)/gm)].flatMap(match => (match[1] ?? '').split(' ')));
  for (const file of ['.github/workflows/ci.yml', '.github/workflows/release.yml', '.github/actions/bootstrap/action.yml']) {
    const text = readFileSync(path.join(ROOT, file), 'utf8');
    for (const [, target = ''] of text.matchAll(/^[ \t]*(?:run: )?make ([a-z][a-z0-9-]*)/gm)) {
      assert.ok(targets.has(target), `${file} runs make ${target}, which the Makefile does not declare`);
    }
  }
});

test('ci.yml has no job that repeats the conclusion of the run', () => {
  const workflow: unknown = parse(readFileSync(path.join(ROOT, '.github/workflows/ci.yml'), 'utf8'));
  const jobs = record(record(workflow).jobs);
  assert.equal(jobs['ci-passed'], undefined, 'ci-passed is deleted: the run already fails when one of its jobs fails');
});
