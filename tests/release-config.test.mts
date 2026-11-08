// Each step of the workflows runs a target of the Makefile of the repository (T69).
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const ROOT = path.join(import.meta.dirname, '..');

test('every run step of the workflows starts a target of the Makefile of the repository', () => {
  const makefile = readFileSync(path.join(ROOT, 'Makefile'), 'utf8');
  const targets = new Set([...makefile.matchAll(/^([a-z][a-z0-9 -]*):(?!=)/gm)].flatMap(match => (match[1] ?? '').split(' ')));
  for (const file of ['.github/workflows/ci.yml', '.github/workflows/release.yml', '.github/actions/bootstrap/action.yml']) {
    const text = readFileSync(path.join(ROOT, file), 'utf8');
    for (const [, target = ''] of text.matchAll(/^[ \t]*(?:run: )?make ([a-z][a-z0-9-]*)/gm)) {
      assert.ok(targets.has(target), `${file} runs make ${target}, which the Makefile does not declare`);
    }
  }
});
