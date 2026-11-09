// The checklist as a corpus (T61, docs/plans/flowmark/plan.md "T61 — the checklist as a corpus"). The corpus
// docs/plans/<plan>/checklist/ holds the rows of the table items of dbspec/checklist.dbs; checklist.md and checklist.ko.md
// are views that this script renders from it, since kit reads only those paths, a corpus file is named
// <table>.flowmark.md, and a state cell of kit holds the state with its cause and retry. The rows are read and written
// only through the flowmark executable, so the engine checks every value: a state, a cause or a retry that the catalog
// refuses is the engine's refusal, never a corrected value.
//
//   node scripts/checklist.mts import <plan> — writes the rows of the current views into the empty corpus, after
//     checking that the rows render the views byte for byte, so the import keeps every byte of them;
//   node scripts/checklist.mts render <plan> — writes the views from the corpus;
//   node scripts/checklist.mts check <plan>… — fails, naming each view, when a view differs from what the corpus renders.
//
// The source-sha256 line of the Korean view is kit's (make documents-stamp writes it, make docs-check checks it), so a
// render keeps the line that the file holds, at its place.
import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

/** The states of a checklist row by name, in the order of config/checklist.json. */
export const STATES = ['waiting', 'active', 'verifying', 'done', 'bypassed', 'impossible'] as const;
export type State = (typeof STATES)[number];
const isState = (value: unknown): value is State => STATES.some((state) => state === value);

/** A row of the table items: its key, its cells in both languages, its order and its state with its cause and retry. */
export interface Item {
  id: string;
  task: string;
  task_ko: string;
  deliverables: string;
  deliverables_ko: string;
  verification: string;
  verification_ko: string;
  position: number;
  state: State;
  cause: string | null;
  cause_ko: string | null;
  retry: string | null;
  retry_ko: string | null;
}

/** The language of a view: its file, the suffix of its columns and the labels of the cause and the retry of its state cell. */
export interface Language {
  file: string;
  suffix: '' | '_ko';
  cause: string;
  retry: string;
}

export const EN: Language = { file: 'checklist.md', suffix: '', cause: 'cause', retry: 'retry' };
export const KO: Language = { file: 'checklist.ko.md', suffix: '_ko', cause: '원인', retry: '재시도' };

/** The columns of a view, as config/checklist.json declares them. */
const COLUMNS = ['ID', 'Task', 'Deliverables', 'Verification', 'State'];

/** The settings of the views that config/checklist.json declares: the marker of each state and the header. */
export interface Tracker {
  markers: Record<State, string>;
  columns: string[];
}

/** The member `key` of the object `value`, or undefined. */
const member = (value: unknown, key: string): unknown => (typeof value === 'object' && value !== null ? Object.entries(value).find(([name]) => name === key)?.[1] : undefined);

/** The tracker of config/checklist.json under `root`; a state or a column that it lacks is an error. */
export function trackerOf(root: string): Tracker {
  const config: unknown = JSON.parse(readFileSync(path.join(root, 'config', 'checklist.json'), 'utf8'));
  const marker = (state: State): string => {
    const value = member(member(config, 'states'), state);
    if (typeof value !== 'string') throw new Error(`config/checklist.json declares no marker for the state ${state}`);
    return value;
  };
  const markers: Record<State, string> = {
    waiting: marker('waiting'), active: marker('active'), verifying: marker('verifying'),
    done: marker('done'), bypassed: marker('bypassed'), impossible: marker('impossible'),
  };
  const columns = member(config, 'columns');
  if (!Array.isArray(columns) || columns.join(' | ') !== COLUMNS.join(' | ')) throw new Error(`config/checklist.json declares the columns ${JSON.stringify(columns)}; the views hold | ${COLUMNS.join(' | ')} |`);
  return { markers, columns: [...COLUMNS] };
}

/** The text of a cell: the pipe is the one character that a cell escapes. */
const escapeCell = (text: string) => text.replaceAll('|', '\\|');

/** The cells of the table row `line`, each unescaped; a line that is no table row is an error. */
export function cellsOf(line: string): string[] {
  if (!line.startsWith('| ') || !line.endsWith(' |')) throw new Error(`the line ${JSON.stringify(line.slice(0, 80))} is no table row`);
  return line.slice(2, -2).split(/(?<!\\) \| /).map((cell) => cell.replaceAll('\\|', '|'));
}

/** The cells of a row of a view: its ID, task, deliverables, verification and state. */
export type Row = [id: string, task: string, deliverables: string, verification: string, state: string];

/** The state cell of `item` in `language`. */
export function stateCell(item: Item, language: Language, tracker: Tracker): string {
  const marker = tracker.markers[item.state];
  const cause = item[`cause${language.suffix}`];
  const retry = item[`retry${language.suffix}`];
  if (item.state === 'bypassed') return `${marker} ${language.cause}: ${cause}; ${language.retry}: ${retry}`;
  if (item.state === 'impossible') return `${marker} ${language.cause}: ${cause}`;
  return marker;
}

/** The lines of a view around its table: those before it (the stamp line apart, with its place) and those after it. */
export interface Frame {
  head: string[];
  stamp: { line: string; at: number } | null;
  tail: string[];
}

const STAMP = /^<!-- source-sha256: [0-9a-f]{64} -->$/;

/** The view of `items` in `language` under `frame`, with the stamp line of the frame when `withStamp`. */
export function render(items: Item[], language: Language, tracker: Tracker, frame: Frame, withStamp = false): string {
  const head = [...frame.head];
  if (withStamp && frame.stamp !== null) head.splice(frame.stamp.at, 0, frame.stamp.line);
  const rows = [...items].sort((a, b) => a.position - b.position).map((item) => {
    const cells = [item.id, item[`task${language.suffix}`], item[`deliverables${language.suffix}`], item[`verification${language.suffix}`]].map(escapeCell);
    return `| ${cells.join(' | ')} | ${escapeCell(stateCell(item, language, tracker))} |`;
  });
  return [...head, `| ${tracker.columns.join(' | ')} |`, `|${tracker.columns.map(() => '---').join('|')}|`, ...rows, ...frame.tail].join('\n');
}

/** The frame and the rows of the view `text`; a view without the table of the header, or a row of other cells, is an error. */
export function parseView(text: string, tracker: Tracker): { frame: Frame; rows: Row[] } {
  const lines = text.split('\n');
  const header = `| ${tracker.columns.join(' | ')} |`;
  const start = lines.indexOf(header);
  if (start < 0 || lines[start + 1] !== `|${tracker.columns.map(() => '---').join('|')}|`) throw new Error(`the view holds no table with the header ${header}`);
  let end = start + 2;
  while (lines[end]?.startsWith('|') === true) end++;
  const head = lines.slice(0, start);
  const at = head.findIndex((line) => STAMP.test(line));
  const stamp = at < 0 ? null : { line: head.splice(at, 1).join(''), at };
  const rows = lines.slice(start + 2, end).map((line): Row => {
    const [id, task, deliverables, verification, state, ...rest] = cellsOf(line);
    if (id === undefined || task === undefined || deliverables === undefined || verification === undefined || state === undefined || rest.length > 0) {
      throw new Error(`the row ${JSON.stringify(line.slice(0, 80))} holds ${cellsOf(line).length} cells; the header holds ${tracker.columns.length}`);
    }
    return [id, task, deliverables, verification, state];
  });
  return { frame: { head, stamp, tail: lines.slice(end) }, rows };
}

/** The state, cause and retry of the state cell `cell` in `language`; a cell that names no state is an error. */
export function parseState(cell: string, language: Language, tracker: Tracker): { state: State; cause: string | null; retry: string | null } {
  for (const state of STATES) {
    const marker = tracker.markers[state];
    if (cell === marker && state !== 'bypassed' && state !== 'impossible') return { state, cause: null, retry: null };
    if (!cell.startsWith(`${marker} `)) continue;
    const rest = cell.slice(marker.length + 1);
    const causeLabel = `${language.cause}: `;
    if (state === 'bypassed' && rest.startsWith(causeLabel)) {
      const retryLabel = `; ${language.retry}: `;
      const retryAt = rest.lastIndexOf(retryLabel);
      if (retryAt >= 0) return { state, cause: rest.slice(causeLabel.length, retryAt), retry: rest.slice(retryAt + retryLabel.length) };
    }
    if (state === 'impossible' && rest.startsWith(causeLabel)) return { state, cause: rest.slice(causeLabel.length), retry: null };
  }
  throw new Error(`the state cell ${JSON.stringify(cell)} is no state of config/checklist.json in the form of ${language.file}`);
}

/** The items that the English view `english` and the Korean view `korean` hold; rows that differ by ID, order or state are an error. */
export function itemsOf(english: string, korean: string, tracker: Tracker): Item[] {
  const en = parseView(english, tracker).rows;
  const ko = parseView(korean, tracker).rows;
  if (en.length !== ko.length) throw new Error(`checklist.md holds ${en.length} rows and checklist.ko.md ${ko.length}`);
  return en.map(([id, task, deliverables, verification, cell], index) => {
    const twin = ko[index];
    if (twin === undefined || twin[0] !== id) throw new Error(`row ${index + 1} is ${id} in checklist.md and ${twin?.[0]} in checklist.ko.md`);
    const [, taskKo, deliverablesKo, verificationKo, cellKo] = twin;
    const state = parseState(cell, EN, tracker);
    const stateKo = parseState(cellKo, KO, tracker);
    if (state.state !== stateKo.state) throw new Error(`${id} is ${state.state} in checklist.md and ${stateKo.state} in checklist.ko.md`);
    return {
      id, task, task_ko: taskKo, deliverables, deliverables_ko: deliverablesKo, verification, verification_ko: verificationKo,
      position: index + 1, state: state.state, cause: state.cause, cause_ko: stateKo.cause, retry: state.retry, retry_ko: stateKo.retry,
    };
  });
}

/** The item of the row `value` that the corpus answered; a value of another shape is an error that names the row. */
export function itemOf(value: unknown): Item {
  const text = (key: string): string => {
    const v = member(value, key);
    if (typeof v !== 'string') throw new Error(`the corpus answered the row ${JSON.stringify(value)} with ${key} not a text`);
    return v;
  };
  const nullable = (key: string): string | null => (member(value, key) === null ? null : text(key));
  const position = member(value, 'position');
  const state = member(value, 'state');
  if (typeof position !== 'number' || !isState(state)) throw new Error(`the corpus answered the row ${JSON.stringify(value)} without a position and a state of the checklist`);
  return {
    id: text('id'), task: text('task'), task_ko: text('task_ko'), deliverables: text('deliverables'), deliverables_ko: text('deliverables_ko'),
    verification: text('verification'), verification_ko: text('verification_ko'), position, state,
    cause: nullable('cause'), cause_ko: nullable('cause_ko'), retry: nullable('retry'), retry_ko: nullable('retry_ko'),
  };
}

/** The answer of the flowmark executable `bin` for `args`, with `input` on its standard input. */
const flowmark = (bin: string, args: string[], input?: string): string => execFileSync(bin, args, { input, encoding: 'utf8', stdio: ['pipe', 'pipe', 'inherit'] });

/** The items of the corpus `corpus`, read by the flowmark executable `bin`. */
export function readCorpus(bin: string, corpus: string): Item[] {
  const rows = member(JSON.parse(flowmark(bin, ['rows', corpus, 'items'])), 'rows');
  if (!Array.isArray(rows)) throw new Error(`flowmark rows ${corpus} items answered no rows`);
  return rows.map(itemOf);
}

function main(argv: string[]): number {
  const [command, ...plans] = argv;
  const root = process.cwd();
  const bin = process.env.FLOWMARK ?? path.join(root, 'build', 'flowmark');
  if (command === undefined || !['import', 'render', 'check'].includes(command) || plans.length === 0) {
    process.stderr.write('usage: node scripts/checklist.mts import|render|check <plan>…\n');
    return 2;
  }
  if (!existsSync(bin)) throw new Error(`${bin} does not exist; make flowmark-build writes it`);
  const tracker = trackerOf(root);
  let failed = 0;
  for (const plan of plans) {
    const folder = path.join(root, 'docs', 'plans', plan);
    const corpus = path.join(folder, 'checklist');
    const viewOf = (language: Language) => {
      const file = path.join(folder, language.file);
      const text = readFileSync(file, 'utf8');
      return { language, file, text, frame: parseView(text, tracker).frame };
    };
    const english = viewOf(EN);
    const korean = viewOf(KO);
    const views = [english, korean];
    if (command === 'import') {
      if (readCorpus(bin, corpus).length > 0) throw new Error(`the corpus ${corpus} holds rows; an import writes only an empty corpus`);
      const items = itemsOf(english.text, korean.text, tracker);
      for (const view of views) {
        if (render(items, view.language, tracker, view.frame, true) !== view.text) throw new Error(`${view.language.file} of ${plan} is not the view of its rows, so an import would change it; write it in the form that the view renders first`);
      }
      flowmark(bin, ['rows.load', corpus, 'items'], JSON.stringify(items));
      process.stdout.write(`[checklist] imported ${items.length} rows of ${plan} into ${path.relative(root, corpus)}\n`);
      continue;
    }
    const items = readCorpus(bin, corpus);
    for (const view of views) {
      const rendered = render(items, view.language, tracker, view.frame, true);
      if (command === 'render') {
        if (rendered !== view.text) writeFileSync(view.file, rendered);
        process.stdout.write(`[checklist] rendered ${path.relative(root, view.file)}\n`);
      } else if (rendered !== view.text) {
        failed++;
        process.stderr.write(`[checklist] ${path.relative(root, view.file)} differs from the view of ${path.relative(root, corpus)}; a row is written through flowmark, and make checklist-render writes the view\n`);
      }
    }
  }
  if (command === 'check' && failed === 0) process.stdout.write(`[checklist] the views of ${plans.join(', ')} are those of their corpora\n`);
  return failed === 0 ? 0 : 1;
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? '').href) process.exitCode = main(process.argv.slice(2));
