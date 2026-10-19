// Tests of the slug-vector recorder (scripts/record-slugs.mts) that need no network: the forms of a code point, the
// headings and the splitting of their ids, the treatments and their compression, and the reading of GitHub's HTML.
import assert from 'node:assert/strict';
import test, { mock } from 'node:test';
import { compress, forms, githubAttributes, githubRender, githubIds, gitlabIds, glfmArgs, groupSize, headingIdAttrs, headingsOf, katexOptions, methodSource, snippetPrefix, splitIds, treatment } from '../scripts/record-slugs.mts';

test('a code point has a reference form and a raw form, escaped when it is ASCII punctuation', () => {
  assert.deepEqual(forms(0x41), { ref: '&#x41;', raw: 'A', htmlRef: '&#x41;', htmlRaw: 'A' });
  assert.deepEqual(forms(0x2a), { ref: '&#x2A;', raw: '\\*', htmlRef: '&#x2A;', htmlRaw: '*' });
  assert.deepEqual(forms(0x0a), { ref: '&#xA;', raw: null, htmlRef: '&#xA;', htmlRaw: null });
  assert.deepEqual(forms(0x0d), { ref: '&#xD;', raw: null, htmlRef: '&#xD;', htmlRaw: null });
  assert.deepEqual(forms(0), { ref: '&#x0;', raw: null, htmlRef: '&#x0;', htmlRaw: null });
  assert.equal(forms(0x3c).htmlRaw, null);
  assert.equal(forms(0x26).htmlRaw, null);
});

test('a raw HTML form is a raw <h2> heading', () => {
  assert.deepEqual(headingsOf([0x41, 0x20], 'htmlRaw').map(h => h.line), ['<h2>s20p q</h2>', '<h2>g1-A-</h2>']);
});

test('the id attribute of every heading element is read', () => {
  assert.deepEqual(headingIdAttrs('<h1 id="a&amp;b">x</h1><H2>y</H2><h3 class="c" id="d">z</h3><h2/>'), ['a&b', null, 'd', null]);
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
  const nested = '<div class="markdown-heading"><h2><b>a<h3>b</h3></b></h2><a id="user-content-ab" class="anchor" href="#ab"></a></div>';
  assert.deepEqual(githubIds(nested), ['ab', null]);
  const unwrapped = '<div class="markdown-heading"><h2>a</h2><h2>b</h2>\n<a id="user-content-ab" class="anchor" href="#ab"></a></div><h2></h2>';
  assert.deepEqual(githubIds(unwrapped), ['ab', null, null]);
  assert.throws(() => githubIds('<a id="user-content-x" class="anchor"></a>'));
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

test("the options of VS Code's markdown-math are read, and an unknown one refused", () => {
  const src = 'const options = {\n\t\t\t\t\tenableFencedBlocks: true,\n\t\t\t\t\tglobalGroup: true,\n\t\t\t\t\tmacros: { ...settingsMacros }\n\t\t\t\t};';
  assert.deepEqual(katexOptions(src), { enableFencedBlocks: true, globalGroup: true, macros: {} });
  assert.throws(() => katexOptions('const options = {\n\tthrowOnError: x\n};'));
});

test("GitLab's engine options become the CLI's flags, and an unknown value is refused", () => {
  const src = "REFERENCE_CHARS = %w[$ % #].freeze\nOPTIONS = {\n  alerts: true,\n  hardbreaks: false,\n  header_id_prefix: Banzai::Renderer::USER_CONTENT_ID_PREFIX,\n  only_escape_chars: REFERENCE_CHARS,\n  math_dollars: true\n}.freeze";
  assert.deepEqual(glfmArgs(src), ['--alerts', '--header-id-prefix', 'user-content-', '--only-escape-chars', '$%#', '--math-dollars']);
  assert.throws(() => glfmArgs('OPTIONS = {\n  smart: maybe\n}.freeze'));
});

test("GitLab's heading ids are read per heading, the prefix removed, raw HTML without an id", () => {
  const html = '<h1 id="user-content-a&amp;b" data-sourcepos="1:1-1:5">a<a href="#a" class="anchor"></a></h1>\n<h2 id="hh">g</h2>\n<h2 id="user-content-" data-sourcepos="3:1-3:2"></h2>';
  assert.deepEqual(gitlabIds(html), ['a&b', 'hh', '']);
  assert.deepEqual(gitlabIds('<h2>g</h2>'), [null]);
  assert.throws(() => gitlabIds('<h1 data-sourcepos="1:1-1:1">x</h1>'));
});

test("a snippet's heading slug prefix is its anchorized base name and a hyphen, as Gitlab::HeadingSlug makes it", () => {
  assert.equal(snippetPrefix('items.flowmark.md'), 'itemsflowmark-');
  assert.equal(snippetPrefix('docs/My Notes.md'), 'my-notes-');
  assert.equal(snippetPrefix('dir/!!!.md'), '');
});

// ghReplies is a request of githubRender, a mock function of node:test that answers the calls in turn from replies:
// each reply is [stderr, exit] for a failure, thrown as execFileSync throws it, or [stdout] for a rendering.
function ghReplies(replies) {
  const request = mock.fn();
  replies.forEach(([out, exit], call) => request.mock.mockImplementationOnce(() => {
    if (exit) throw Object.assign(new Error('Command failed: gh api -X POST /markdown --input -'), { status: exit, stderr: Buffer.from(out) });
    return out;
  }, call));
  return request;
}

const noResponse = ['Post "https://api.github.com/markdown": dial tcp 20.200.245.245:443: connect: operation timed out\n', 1];

test('a GitHub request whose connection fails before a response is sent again', () => {
  const gh = ghReplies([noResponse, ['<p>x</p>\n']]);
  assert.equal(githubRender('x', gh), '<p>x</p>\n');
  assert.equal(gh.mock.callCount(), 2);
});

test('a GitHub request that gets an HTTP error is not sent again', () => {
  const gh = ghReplies([['HTTP 422: Validation Failed\n', 1], ['<p>x</p>\n']]);
  assert.throws(() => githubRender('x', gh), /Command failed/);
  assert.equal(gh.mock.callCount(), 1);
});

test('a GitHub request whose connection fails at every attempt ends the recording', () => {
  const gh = ghReplies([noResponse, noResponse, noResponse, ['<p>x</p>\n']]);
  assert.throws(() => githubRender('x', gh), /Command failed/);
  assert.equal(gh.mock.callCount(), 3);
});
