// The views of the checklist corpus (T61): scripts/checklist.mts reads a view into its rows and renders the rows into the
// view byte for byte, and refuses a view that no rows render.
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { cellsOf, EN as en, itemOf, itemsOf, KO as ko, parseState, parseView, render, type Item, type Tracker } from '../scripts/checklist.mts';

const tracker: Tracker = {
  markers: { waiting: '[ ]', active: '[~]', verifying: '[v]', done: '[o]', bypassed: '[!]', impossible: '[-]' },
  columns: ['ID', 'Task', 'Deliverables', 'Verification', 'State'],
};
const head = '| ID | Task | Deliverables | Verification | State |\n|---|---|---|---|---|';

const english = `<!-- doc-id: execution-checklist -->
# Execution checklist

${head}
| T1 | Read \`a \\| b\` | - | - | [o] |
| T2 | Wait | x | y | [ ] |
| T3 | Hold | - | - | [!] cause: no runner; retry: a runner |
| T4 | Drop | - | - | [-] cause: no platform |
`;
const korean = `<!-- doc-id: execution-checklist -->
<!-- source-sha256: ${'0'.repeat(64)} -->
# 실행 체크리스트

${head}
| T1 | \`a \\| b\` 읽기 | - | - | [o] |
| T2 | 대기 | x | y | [ ] |
| T3 | 보류 | - | - | [!] 원인: runner 없음; 재시도: runner |
| T4 | 제외 | - | - | [-] 원인: platform 없음 |
`;

test('the rows of both views render each view byte for byte, the stamp of the Korean view kept', () => {
  const items = itemsOf(english, korean, tracker);
  assert.equal(items.length, 4);
  assert.deepEqual(items.map((item) => [item.id, item.position, item.state]), [['T1', 1, 'done'], ['T2', 2, 'waiting'], ['T3', 3, 'bypassed'], ['T4', 4, 'impossible']]);
  const [read, , hold, drop] = items;
  assert.equal(read?.task, 'Read `a | b`');
  assert.deepEqual([hold?.cause, hold?.retry, hold?.cause_ko, hold?.retry_ko], ['no runner', 'a runner', 'runner 없음', 'runner']);
  assert.deepEqual([drop?.cause, drop?.retry, drop?.cause_ko, drop?.retry_ko], ['no platform', null, 'platform 없음', null]);
  for (const [text, language] of [[english, en], [korean, ko]] as const) assert.equal(render(items, language, tracker, parseView(text, tracker).frame, true), text);
});

test('a view renders its rows in their order, whatever order they are given in', () => {
  const items = itemsOf(english, korean, tracker).reverse();
  assert.equal(render(items, en, tracker, parseView(english, tracker).frame, true), english);
});

test('a state cell is read in the labels of its language and refused in any other form', () => {
  assert.deepEqual(parseState('[v]', en, tracker), { state: 'verifying', cause: null, retry: null });
  assert.deepEqual(parseState('[!] cause: a; b; retry: c', en, tracker), { state: 'bypassed', cause: 'a; b', retry: 'c' });
  assert.deepEqual(parseState('[-] 원인: a', ko, tracker), { state: 'impossible', cause: 'a', retry: null });
  for (const cell of ['[x]', '[o] done', '[!] cause: a', '[!] 원인: a; 재시도: b', '[-]', '[!]', '[v]x']) assert.throws(() => parseState(cell, en, tracker), /is no state/, cell);
});

test('a row whose cells are not those of the header, and a line that is no row, are refused', () => {
  assert.deepEqual(cellsOf('| a | b \\| c |'), ['a', 'b | c']);
  assert.throws(() => cellsOf('|a | b |'), /no table row/);
  assert.throws(() => parseView(`${head}\n| T1 | a | - | [o] |\n`, tracker), /holds 4 cells/);
  assert.throws(() => parseView('# Execution checklist\n', tracker), /no table with the header/);
});

test('views that differ by a row, its ID or its state are refused', () => {
  const drop = korean.replace(/^\| T4 .*\n/m, '');
  assert.throws(() => itemsOf(english, drop, tracker), /holds 4 rows and checklist.ko.md 3/);
  assert.throws(() => itemsOf(english, korean.replace('| T2 |', '| T9 |'), tracker), /row 2 is T2 in checklist.md and T9/);
  assert.throws(() => itemsOf(english, korean.replace('| 대기 | x | y | [ ] |', '| 대기 | x | y | [~] |'), tracker), /T2 is waiting in checklist.md and active/);
});

test('a row that the corpus answers in another shape is refused, never read as a row', () => {
  const row: Item = { id: 'T1', task: 'a', task_ko: 'a', deliverables: '-', deliverables_ko: '-', verification: '-', verification_ko: '-', position: 1, state: 'done', cause: null, cause_ko: null, retry: null, retry_ko: null };
  assert.deepEqual(itemOf(row), row);
  assert.throws(() => itemOf({ ...row, state: 'closed' }), /without a position and a state/);
  assert.throws(() => itemOf({ ...row, position: '1' }), /without a position and a state/);
  assert.throws(() => itemOf({ ...row, task: null }), /task not a text/);
  assert.throws(() => itemOf({ ...row, cause: 1 }), /cause not a text/);
});
