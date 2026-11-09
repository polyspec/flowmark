// Tests of the run check of a release (scripts/check-release-run.mts, T90): a release takes a successful push or manual run of
// .github/workflows/ci.yml on the commit it tags, and refuses a run of any other workflow. The fixtures are answers of
// gh api repos/<repo>/actions/runs/<id>, cut to the fields that the check reads.
import assert from 'node:assert/strict';
import test from 'node:test';
import { repositoryOf, runErrors } from '../scripts/check-release-run.mts';

const SHA = 'c'.repeat(40);
const run = (fields: Record<string, unknown>): Record<string, unknown> => ({ id: 7, name: 'ci', path: '.github/workflows/ci.yml', event: 'push', status: 'completed', conclusion: 'success', head_sha: SHA, head_branch: 'main', ...fields });

test('a successful push run of ci.yml is accepted, also when it ran on the commit that the release tags', () => {
  assert.deepEqual(runErrors('7', run({})), []);
  assert.deepEqual(runErrors('7', run({}), SHA), []);
});

test('a successful manual run of ci.yml is accepted', () => {
  assert.deepEqual(runErrors('7', run({ event: 'workflow_dispatch' })), []);
});

test('a run of documents.yml is refused and names its workflow', () => {
  assert.deepEqual(runErrors('8', run({ name: 'documents', path: '.github/workflows/documents.yml' })), [
    'run 8 is a run of .github/workflows/documents.yml, not of .github/workflows/ci.yml; a release takes a successful run of .github/workflows/ci.yml',
  ]);
});

test('a run of docs-pages.yml is refused and names its workflow', () => {
  assert.deepEqual(runErrors('9', run({ name: 'docs-pages', path: '.github/workflows/docs-pages.yml' })), [
    'run 9 is a run of .github/workflows/docs-pages.yml, not of .github/workflows/ci.yml; a release takes a successful run of .github/workflows/ci.yml',
  ]);
});

test('a ci.yml run of another event, a failed one and one on another commit are refused', () => {
  assert.deepEqual(runErrors('7', run({ event: 'pull_request' })), ['run 7 is a pull_request run, not a push or workflow_dispatch run']);
  assert.deepEqual(runErrors('7', run({ conclusion: 'failure' })), ['run 7 has the conclusion failure, not success']);
  assert.deepEqual(runErrors('7', run({ conclusion: null })), ['run 7 has the conclusion null, not success']);
  assert.deepEqual(runErrors('7', run({}), 'd'.repeat(40)), [`run 7 ran on ${SHA}, not on the commit ${'d'.repeat(40)} that the release tags`]);
  assert.deepEqual(runErrors('7', run({ head_sha: undefined })), ['run 7 names no head_sha']);
});

test('an answer that is no run is refused on every field', () => {
  assert.equal(runErrors('7', { message: 'Not Found' }).length, 4);
});

test('the repository is the owner and name of the repositoryUrl of config/release.json', () => {
  assert.equal(repositoryOf({ repositoryUrl: 'https://github.com/polyspec/flowmark' }), 'polyspec/flowmark');
  assert.throws(() => repositoryOf({ repositoryUrl: 'git@github.com:polyspec/flowmark.git' }), /no repositoryUrl/);
});
