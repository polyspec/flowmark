// Tests of config/release.json: release-verify requires each named check on the tagged commit, and GitHub names a
// check run after its job, so every named check is a job of a workflow of the repository.
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { parse } from 'yaml';

const ROOT = path.join(import.meta.dirname, '..');
const WORKFLOWS = path.join(ROOT, '.github/workflows');

interface Workflow {
  jobs?: Record<string, { name?: string }>;
}

/** The check run names of the jobs of every workflow: the job's name, or its ID when it has none. */
function checkRuns(): Set<string> {
  const names = new Set<string>();
  for (const file of readdirSync(WORKFLOWS).filter((name) => /\.ya?ml$/.test(name))) {
    const workflow = parse(readFileSync(path.join(WORKFLOWS, file), 'utf8')) as Workflow;
    for (const [id, job] of Object.entries(workflow.jobs ?? {})) names.add(job.name ?? id);
  }
  return names;
}

test('every check that config/release.json requires is the check run of a workflow job', () => {
  const { checks } = JSON.parse(readFileSync(path.join(ROOT, 'config/release.json'), 'utf8')) as { checks: string[] };
  const runs = checkRuns();
  assert.deepEqual(
    checks.filter((check) => !runs.has(check)),
    [],
  );
});
