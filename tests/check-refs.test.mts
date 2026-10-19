// Tests of the reference check (scripts/check-refs.mts): every task ID that a selected document names in prose resolves
// to an item of a plan's checklist, and prose excludes fenced blocks and inline code.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { checkReferences } from '../scripts/check-refs.mts';

const CHECKLIST = { schema: 1, hooks: ['pre-push', 'commit-msg'], idPattern: 'T[0-9]+(?:[.-][0-9]+)*' };
const DOCUMENTS = { schema: 1, include: ['**/*.md'], exclude: ['tests/kit/**'] };
const LIST = '# List checklist\n\n| ID | Task | Deliverables | Verification | State |\n|---|---|---|---|---|\n| T1 | Write the parser | - | - | [o] |\n| T2 | Print the union | - | - | [ ] |\n';

interface Checkout {
  root: string;
  dispose: () => void;
}

/** A git checkout of `files` (path to text), with its configuration; the files are new and not ignored, as trackedFiles lists them. */
function checkout(files: Record<string, string>, ignore = ''): Checkout {
  const root = mkdtempSync(path.join(tmpdir(), 'check-refs-'));
  const base: Record<string, string> = {
    'config/checklist.json': JSON.stringify(CHECKLIST),
    'config/documents.json': JSON.stringify(DOCUMENTS),
    '.gitignore': ignore,
    ...files,
  };
  for (const [name, text] of Object.entries(base)) {
    mkdirSync(path.dirname(path.join(root, name)), { recursive: true });
    writeFileSync(path.join(root, name), text);
  }
  execFileSync('git', ['init', '-q'], { cwd: root });
  return { root, dispose: () => rmSync(root, { recursive: true, force: true }) };
}

const findingsOf = (result: ReturnType<typeof checkReferences>): unknown[][] =>
  result.findings.map(({ file, line, column, rule, message }) => [file, line, column, rule, message]);

test('a task ID that names no checklist item is a finding with its place', () => {
  const { root, dispose } = checkout({
    'docs/plans/list/checklist.md': LIST,
    'docs/spec/a.md': '# A\n\nDone T1. And T9.9 names no item.\n',
  });
  try {
    const result = checkReferences(root);
    assert.deepEqual(findingsOf(result), [['docs/spec/a.md', 3, 14, 'refs-unresolved', 'the task T9.9 names no checklist item']]);
    assert.equal(result.references, 4);
  } finally {
    dispose();
  }
});

test('an item of any plan folder resolves, and a document that Git ignores is not read', () => {
  const { root, dispose } = checkout(
    {
      'docs/plans/list/checklist.md': LIST,
      'docs/copy/a.md': '# A\n\nT9.9 names no item.\n',
      'docs/spec/b.md': '# B\n\nT2 is open, T8.8 names no item.\n',
    },
    'docs/copy/\n',
  );
  try {
    const result = checkReferences(root);
    assert.deepEqual(findingsOf(result), [['docs/spec/b.md', 3, 13, 'refs-unresolved', 'the task T8.8 names no checklist item']]);
    assert.equal(result.files, 2);
    assert.equal(result.references, 4);
  } finally {
    dispose();
  }
});

test('a task ID inside inline code or a fenced block is not a reference', () => {
  const { root, dispose } = checkout({
    'docs/plans/list/checklist.md': LIST,
    'docs/spec/a.md': '# A\n\nThe shape `T9.9` is an example, and:\n\n```\nT9.9 in a fence\n```\n',
  });
  try {
    const result = checkReferences(root);
    assert.deepEqual(findingsOf(result), []);
    assert.equal(result.references, 2);
  } finally {
    dispose();
  }
});
