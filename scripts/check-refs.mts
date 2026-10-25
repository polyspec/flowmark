// The reference check: every task ID that a document selected by config/documents.json names in its prose resolves to an
// item of a plan's checklist, docs/plans/<plan>/checklist.md. Prose excludes fenced blocks and inline code spans, so a
// grammar example is not a reference. It reads the files that trackedFiles of @polyspec/kit lists, the tracked files and
// the new files that Git does not ignore. The checklist format and the document selection are read here, so the script
// depends on the one export of kit that it needs. Findings carry { file, line, column, rule, message } like the kit checks.
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { trackedFiles } from '@polyspec/kit/tracked-files';

interface Finding {
  file: string;
  line?: number;
  column?: number;
  rule: string;
  message: string;
}

export interface ReferenceResult {
  files: number;
  references: number;
  findings: Finding[];
}

interface ChecklistConfig {
  idPattern: string;
}

interface DocumentsConfig {
  include: string[];
  exclude?: string[];
}

const isStrings = (v: unknown): v is string[] => Array.isArray(v) && v.every(x => typeof x === 'string');

/** config/checklist.json as check-refs reads it: its idPattern, a string; any other shape is an error naming it. */
function checklistConfig(v: unknown): ChecklistConfig {
  if (typeof v !== 'object' || v === null || !('idPattern' in v) || typeof v.idPattern !== 'string') throw new Error('config/checklist.json has no idPattern string');
  return { idPattern: v.idPattern };
}

/** config/documents.json as check-refs reads it: include, and exclude when given, lists of globs. */
function documentsConfig(v: unknown): DocumentsConfig {
  if (typeof v !== 'object' || v === null || !('include' in v) || !isStrings(v.include)) throw new Error('config/documents.json has no include list of globs');
  if (!('exclude' in v)) return { include: v.include };
  if (!isStrings(v.exclude)) throw new Error('config/documents.json has an exclude that is no list of globs');
  return { include: v.include, exclude: v.exclude };
}

const CHECKLIST = /^docs\/plans\/[^/]+\/checklist\.md$/;

/** The regular expression of a glob of config/documents.json: `**` crosses folders, `*` and `?` do not. */
export function globToRegExp(glob: string): RegExp {
  let source = '';
  for (let index = 0; index < glob.length; index++) {
    const character = glob[index];
    if (character === '*' && glob[index + 1] === '*') {
      index++;
      if (glob[index + 1] === '/') {
        index++;
        source += '(?:.*/)?';
      } else {
        source += '.*';
      }
    } else if (character === '*') {
      source += '[^/]*';
    } else if (character === '?') {
      source += '[^/]';
    } else {
      source += (character ?? '').replace(/[.+^${}()|[\]\\]/g, '\\$&');
    }
  }
  return new RegExp(`^${source}$`);
}

/** Whether `file` matches one of the globs. */
export function matchesAny(globs: string[], file: string): boolean {
  return globs.some(glob => globToRegExp(glob).test(file));
}

/**
 * The lines of a Markdown text that are prose: fenced blocks become empty lines, and each inline code span becomes spaces
 * of its own length, so the columns of the other text stay where they are. Returns each line with its number (1-based).
 */
export function proseLines(text: string): { line: number; text: string }[] {
  const lines: { line: number; text: string }[] = [];
  let fence: string | undefined;
  text.split('\n').forEach((raw, index) => {
    const opener = /^ {0,3}(`{3,}|~{3,})/.exec(raw)?.[1];
    if (fence !== undefined) {
      const closer = /^ {0,3}(`{3,}|~{3,})\s*$/.exec(raw)?.[1];
      if (closer !== undefined && closer[0] === fence[0] && closer.length >= fence.length) fence = undefined;
      lines.push({ line: index + 1, text: '' });
      return;
    }
    if (opener !== undefined) {
      fence = opener;
      lines.push({ line: index + 1, text: '' });
      return;
    }
    const spans = raw.replace(/(`+)(?!`)[\s\S]*?(?<!`)\1(?!`)/g, match => ' '.repeat(match.length));
    lines.push({ line: index + 1, text: spans });
  });
  return lines;
}

/** The IDs of the items of every checklist of a plan folder: the first cell of each table row whose ID matches. */
function itemIds(root: string, files: string[], idPattern: string): Set<string> {
  const id = new RegExp(`^(?:${idPattern})$`);
  const ids = new Set<string>();
  for (const file of files.filter(item => CHECKLIST.test(item))) {
    for (const line of readFileSync(path.join(root, file), 'utf8').split('\n')) {
      const trimmed = line.trim();
      if (!trimmed.startsWith('|')) continue;
      const first = trimmed.split('|')[1]?.trim() ?? '';
      if (id.test(first)) ids.add(first);
    }
  }
  return ids;
}

/**
 * Reads the checklists of the checkout `root` and the prose of every selected document. Returns how many documents and
 * task-ID references it read, and a finding for each reference that resolves to no item.
 */
export function checkReferences(root: string): ReferenceResult {
  const config = checklistConfig(JSON.parse(readFileSync(path.join(root, 'config', 'checklist.json'), 'utf8')));
  const documents = documentsConfig(JSON.parse(readFileSync(path.join(root, 'config', 'documents.json'), 'utf8')));
  const files = trackedFiles(root);
  const ids = itemIds(root, files, config.idPattern);
  const pattern = new RegExp(`(?<![\\w.])(?:${config.idPattern})(?![\\w-])(?!\\.\\d)`, 'g');
  const findings: Finding[] = [];
  let read = 0;
  let references = 0;
  for (const file of files) {
    if (!file.endsWith('.md') || !matchesAny(documents.include, file) || matchesAny(documents.exclude ?? [], file)) continue;
    read += 1;
    for (const line of proseLines(readFileSync(path.join(root, file), 'utf8'))) {
      for (const match of line.text.matchAll(pattern)) {
        references += 1;
        if (!ids.has(match[0])) {
          findings.push({
            file,
            line: line.line,
            column: (match.index ?? 0) + 1,
            rule: 'refs-unresolved',
            message: `the task ${match[0]} names no checklist item`,
          });
        }
      }
    }
  }
  return { files: read, references, findings };
}

const sentence = (finding: Finding): string => `${finding.file}: ${finding.line ? `line ${finding.line}: ` : ''}${finding.message}`;

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const { files, references, findings } = checkReferences(process.cwd());
  for (const finding of findings) console.log(`[check-refs] ${sentence(finding)}`);
  if (findings.length > 0) {
    console.log(`[check-refs] ${findings.length} findings in ${files} documents`);
    process.exit(1);
  }
  console.log(`[check-refs] ${references} references across ${files} documents resolve`);
}
