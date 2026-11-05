// The internal-path check (K1002): a part of flowmark reaches a dependency only through its installed package and the
// entry point that the package declares, never through a path inside the dependency's own source or build output. A
// line of a tracked file that names `node_modules/<pkg>/dist/` or `node_modules/<pkg>/src/`, or `vendor/polyspec/<pkg>/src/`
// or `vendor/polyspec/<pkg>/dist/`, is a finding; a package's declared bin, such as node_modules/.bin/orm-gen, is not.
// It reads the files that trackedFiles of @polyspec/kit lists, and skips binary files and this script itself, whose
// patterns are written out here. Findings carry { file, line, rule, message } like the other checks of this repository.
import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import { trackedFiles } from '@polyspec/kit/tracked-files';

export interface Finding {
  file: string;
  line: number;
  rule: string;
  message: string;
}

const SELF = 'scripts/check-internal-paths.mts';
const RULE = 'internal-path';

// A dependency's own source or build directory, named by its package path. A scope and a plain package name both match.
const PATTERNS: RegExp[] = [
  /node_modules\/(?:@[\w.-]+\/)?[\w.-]+\/(?:dist|src)\//,
  /vendor\/polyspec\/[\w.-]+\/(?:dist|src)\//,
];

/** The internal-path findings of one text, one per line that names such a path. */
export function scanText(file: string, text: string): Finding[] {
  const findings: Finding[] = [];
  text.split('\n').forEach((line, index) => {
    if (PATTERNS.some(pattern => pattern.test(line))) {
      findings.push({
        file,
        line: index + 1,
        rule: RULE,
        message: 'names the source or build output of a dependency by its path; call the declared bin or entry point instead (K1002)',
      });
    }
  });
  return findings;
}

/** The internal-path findings of the checkout under `root`: every tracked text file except this script. */
export function checkInternalPaths(root: string): { files: number; findings: Finding[] } {
  const files = trackedFiles(root).filter(file => file !== SELF);
  const findings: Finding[] = [];
  for (const file of files) {
    const buffer = readFileSync(`${root}/${file}`);
    if (buffer.includes(0)) continue;
    findings.push(...scanText(file, buffer.toString('utf8')));
  }
  return { files: files.length, findings };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const { files, findings } = checkInternalPaths(process.cwd());
  for (const finding of findings) console.log(`[internal-paths-check] ${finding.file}:${finding.line}: ${finding.message}`);
  if (findings.length > 0) {
    console.log(`[internal-paths-check] ${findings.length} findings in ${files} files`);
    process.exit(1);
  }
  console.log(`[internal-paths-check] no tracked file names a dependency's source or build output by its path (${files} files)`);
}
