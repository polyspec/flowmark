// Tests of the reference check (scripts/check-refs.mjs): every task ID that a selected document names in prose resolves
// to an item of a checklist, and prose excludes fenced blocks and inline code.
import assert from 'node:assert/strict';
import { cpSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { checkReferences } from '../scripts/check-refs.mjs';

const HERE = path.dirname(fileURLToPath(import.meta.url));

const CHECKLIST = {
  schema: 1,
  hooks: ['pre-push'],
  trackers: [{ path: 'docs/plans/list.md', format: 'list', idPattern: 'T[0-9]+(?:[.-][0-9]+)*', states: ['[ ]', '[~]', '[o]', '[!]', '[-]'], active: '[~]' }],
};
const DOCUMENTS = { schema: 1, include: ['docs/**/*.md'], changelogs: [] };

function checkout(t, files) {
  const root = mkdtempSync(path.join(tmpdir(), 'check-refs-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  mkdirSync(path.join(root, 'scripts/kit/schema'), { recursive: true });
  cpSync(path.join(HERE, '../scripts/kit/schema/checklist.schema.json'), path.join(root, 'scripts/kit/schema/checklist.schema.json'));
  for (const [name, text] of Object.entries(files)) {
    mkdirSync(path.dirname(path.join(root, name)), { recursive: true });
    writeFileSync(path.join(root, name), text);
  }
  return root;
}

const references = found => found.findings.map(({ file, line, column, rule, message }) => [file, line, column, rule, message]);

test('a task ID that names no checklist item is a finding with its place', (t) => {
  const root = checkout(t, {
    'config/checklist.json': JSON.stringify(CHECKLIST),
    'config/documents.json': JSON.stringify(DOCUMENTS),
    'docs/plans/list.md': '# List\n\n- [o] T1 Write the parser.\n- [~] T2 Print the union.\n',
    'docs/spec/a.md': '# A\n\nDone T1. And T9.9 names no item.\n',
  });
  const found = checkReferences(root);
  assert.deepEqual(references(found), [['docs/spec/a.md', 3, 14, 'refs-unresolved', 'the task T9.9 names no checklist item']]);
  assert.equal(found.references, 4);
});

test('a task ID inside inline code or a fenced block is not a reference', (t) => {
  const root = checkout(t, {
    'config/checklist.json': JSON.stringify(CHECKLIST),
    'config/documents.json': JSON.stringify(DOCUMENTS),
    'docs/plans/list.md': '# List\n\n- [o] T1 Write the parser.\n',
    'docs/spec/a.md': '# A\n\nThe shape `T9.9` is an example, and:\n\n```\nT9.9 in a fence\n```\n',
  });
  const found = checkReferences(root);
  assert.deepEqual(references(found), []);
  assert.equal(found.references, 1);
});

test('a tracker that cannot be read is a finding of its own document', (t) => {
  const root = checkout(t, {
    'config/checklist.json': JSON.stringify(CHECKLIST),
    'config/documents.json': JSON.stringify(DOCUMENTS),
    'docs/plans/list.md': '# List\n\n- [o] T1 Write the parser.\n- [x] T2 Broken state.\n',
  });
  const found = checkReferences(root);
  assert.match(references(found).at(-1).at(-1), /T2/);
});
