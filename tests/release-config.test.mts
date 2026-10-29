// Tests of config/release.json: release-verify requires each named check on the tagged commit, and GitHub names a
// check run after its job, so every named check is a job of a workflow of the repository.
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { parse } from 'yaml';

const ROOT = path.join(import.meta.dirname, '..');
const WORKFLOWS = path.join(ROOT, '.github/workflows');

/** The record that `value` is, or an empty one. */
function record(value: unknown): Record<string, unknown> {
  return typeof value === 'object' && value !== null ? Object.fromEntries(Object.entries(value)) : {};
}

/** The check run names of the jobs of every workflow: the job's name, or its ID when it has none. */
function checkRuns(): Set<string> {
  const names = new Set<string>();
  for (const file of readdirSync(WORKFLOWS).filter((name) => /\.ya?ml$/.test(name))) {
    const workflow: unknown = parse(readFileSync(path.join(WORKFLOWS, file), 'utf8'));
    for (const [id, job] of Object.entries(record(record(workflow).jobs))) {
      const { name } = record(job);
      names.add(typeof name === 'string' ? name : id);
    }
  }
  return names;
}

test('every check that config/release.json requires is the check run of a workflow job', () => {
  const config: unknown = JSON.parse(readFileSync(path.join(ROOT, 'config/release.json'), 'utf8'));
  const { checks } = record(config);
  assert.ok(Array.isArray(checks));
  const runs = checkRuns();
  assert.deepEqual(
    checks.filter((check) => typeof check !== 'string' || !runs.has(check)),
    [],
  );
});
