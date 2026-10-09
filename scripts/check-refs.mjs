// The reference check: every task ID that a document selected by config/documents.json names in its prose resolves to an
// item of a checklist of config/checklist.json. Prose excludes fenced blocks and inline code spans, so a grammar example
// is not a reference. A stopgap until the checklist application reads its own corpus; the findings carry
// { file, line, column, rule, message } like the kit checks.
import { readFileSync, readdirSync } from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { loadConfig, readChecklist } from './kit/checklist.mjs';
import { proseLines } from './kit/markdown.mjs';
import { matchesAny } from './kit/glob.mjs';
import { readJson } from './kit/files.mjs';

const walk = (root, dir = '') => readdirSync(path.join(root, dir), { withFileTypes: true })
  .flatMap(entry => (entry.isDirectory() ? walk(root, path.join(dir, entry.name)) : [path.join(dir, entry.name).replaceAll('\\', '/')]));

/**
 * Reads the checklists of `root` and the prose of every selected document and returns
 * `{ files, references, findings }`: how many documents and task-ID references it read, and a finding
 * `{ file, line, column, rule, message }` for each reference that resolves to no item and for each
 * checklist it cannot read.
 */
export function checkReferences(root) {
  const config = loadConfig(root);
  const documents = readJson(root, 'config', 'documents.json');
  const findings = [];
  const ids = new Set();
  for (const tracker of config.trackers) {
    const read = readChecklist(readFileSync(path.join(root, tracker.path), 'utf8'), tracker);
    for (const found of read.findings) findings.push({ file: tracker.path, ...found });
    for (const item of read.items) ids.add(item.id);
  }
  const patterns = config.trackers.map(tracker => new RegExp(`(?<![\\w.])(?:${tracker.idPattern})(?![\\w-])(?!\\.\\d)`, 'g'));
  let files = 0;
  let references = 0;
  for (const file of walk(root)) {
    if (!file.endsWith('.md') || !matchesAny(documents.include, file) || matchesAny(documents.exclude ?? [], file)) continue;
    files += 1;
    for (const line of proseLines(readFileSync(path.join(root, file), 'utf8'))) {
      for (const pattern of patterns) {
        for (const match of line.text.matchAll(pattern)) {
          references += 1;
          if (!ids.has(match[0])) findings.push({ file, line: line.line, column: match.index + 1, rule: 'refs-unresolved', message: `the task ${match[0]} names no checklist item` });
        }
      }
    }
  }
  return { files, references, findings };
}

const sentence = finding => `${finding.file}: ${finding.line ? `line ${finding.line}: ` : ''}${finding.message}`;

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const { files, references, findings } = checkReferences(process.cwd());
  for (const finding of findings) console.log(`[check-refs] ${sentence(finding)}`);
  if (findings.length > 0) {
    console.log(`[check-refs] ${findings.length} findings in ${files} documents`);
    process.exit(1);
  }
  console.log(`[check-refs] ${references} references across ${files} documents resolve`);
}
