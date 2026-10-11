// Tests of the slug-vector recorder (scripts/record-slugs.mjs) that need no network: the forms of a code point, the
// headings and the splitting of their ids, the treatments and their compression, and the reading of GitHub's HTML.
import assert from 'node:assert/strict';
import test from 'node:test';
import { compress, forms, githubAttributes, githubIds, groupSize, headingsOf, methodSource, splitIds, treatment } from '../scripts/record-slugs.mjs';

test('a code point has a reference form and a raw form, escaped when it is ASCII punctuation', () => {
  assert.deepEqual(forms(0x41), { ref: '&#x41;', raw: 'A' });
  assert.deepEqual(forms(0x2a), { ref: '&#x2A;', raw: '\\*' });
  assert.deepEqual(forms(0x0a), { ref: '&#xA;', raw: null });
  assert.deepEqual(forms(0x0d), { ref: '&#xD;', raw: null });
  assert.deepEqual(forms(0), { ref: '&#x0;', raw: null });
});

test('the space and the hyphen are read alone, the rest in groups', () => {
  const hs = headingsOf([0x20, 0x41, 0x2d, 0x42], 'raw');
  assert.deepEqual(hs.map(h => h.line), ['# s20p q', '# s2dp\\-q', '# g2-A-B-']);
  assert.equal(headingsOf(Array.from({ length: groupSize + 1 }, (_, i) => 0x4e00 + i), 'ref').length, 2);
});

test('a group id splits at its hyphens, and one that does not split is read again', () => {
  const [h] = headingsOf([0x41, 0x2e], 'raw');
  assert.deepEqual([...splitIds(h, 'g0-a--').ids], [[0x41, 'a'], [0x2e, '']]);
  assert.equal(splitIds(h, 'g0-a-b--').again, true);
  const [s] = headingsOf([0x20], 'ref');
  assert.deepEqual([...splitIds(s, 's20p-q').ids], [[0x20, '-']]);
  assert.throws(() => splitIds(s, 'x'));
});

test('a treatment names the id relative to the code point', () => {
  assert.equal(treatment(0x41, 'a'), '+20');
  assert.equal(treatment(0x61, 'a'), 'keep');
  assert.equal(treatment(0x21, ''), 'drop');
  assert.equal(treatment(0x130, 'i̇'), '0069 0307');
  assert.equal(treatment(0x0a, null), 'none');
  assert.equal(treatment(0x2c, ')'), '-3');
});

test('compression joins runs across the surrogates', () => {
  const ref = new Map();
  const raw = new Map();
  for (let cp = 0; cp <= 0x10ffff; cp++) {
    if (cp >= 0xd800 && cp <= 0xdfff) continue;
    ref.set(cp, String.fromCodePoint(cp));
    raw.set(cp, cp === 0x41 ? '' : String.fromCodePoint(cp));
  }
  assert.deepEqual(compress(ref, raw), ['0000..0040 ; keep ; keep', '0041..0041 ; keep ; drop', '0042..10FFFF ; keep ; keep']);
});

test("GitHub's heading ids are read per heading, the prefix removed", () => {
  const html = '<h1></h1>\n<div class="markdown-heading"><h1 class="heading-element">A&amp;B</h1><a id="user-content-ab" class="anchor" href="#ab"></a></div>\n'
    + '<div class="markdown-heading"><h2 class="heading-element">!</h2><a id="" class="anchor" href="#"></a></div>';
  assert.deepEqual(githubIds(html), [null, 'ab', '']);
});

test("GitHub's raw id and name attributes are read, its heading anchors are not", () => {
  const html = '<p><a id="user-content-x">y</a> <a name="user-content-&amp;">z</a></p><a id="user-content-h" class="anchor"></a>';
  assert.deepEqual(githubAttributes(html), [{ attr: 'id', value: 'user-content-x' }, { attr: 'name', value: 'user-content-&' }]);
});

test('a method is taken to its matching brace', () => {
  const src = 'class A {\n\t#m(x: T): string {\n\t\tif (x) { return 1; }\n\t}\n\tn() {}\n}';
  assert.equal(methodSource(src, '#m(x: T): string {'), '#m(x: T): string {\n\t\tif (x) { return 1; }\n\t}');
  assert.throws(() => methodSource(src, '#z('));
});
